package daraja

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// B2CConfig is the business-to-customer side of the app: the shortcode payouts leave from, the initiator allowed to
// send them, and where Daraja posts payment results and the Transaction Status answers about payouts (apart from the
// paybill's, so each module receives its own).
type B2CConfig struct {
	ShortCode          string
	Initiator          string
	SecurityCredential string // a secret: the initiator's password encrypted with Safaricom's certificate
	ResultURL          string
	TimeoutURL         string
	StatusResultURL    string
	StatusTimeoutURL   string
}

func (c B2CConfig) validate(production bool) error {
	for name, v := range map[string]string{
		"b2c shortcode": c.ShortCode, "initiator": c.Initiator, "security credential": c.SecurityCredential,
		"b2c result url": c.ResultURL, "b2c timeout url": c.TimeoutURL, "b2c status result url": c.StatusResultURL,
		"b2c status timeout url": c.StatusTimeoutURL,
	} {
		if v == "" {
			return fmt.Errorf("daraja: %s is required", name)
		}
	}
	for _, raw := range []string{c.ResultURL, c.TimeoutURL, c.StatusResultURL, c.StatusTimeoutURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return errors.New("daraja: b2c urls must be absolute")
		}
		if production && u.Scheme != "https" {
			return errors.New("daraja: Safaricom calls back over https only")
		}
	}
	return nil
}

// WithB2C enables payouts. Without it the B2C calls return ErrInvalidRequest.
func (c *Client) WithB2C(cfg B2CConfig) (*Client, error) {
	base, _ := url.Parse(c.cfg.BaseURL)
	if err := cfg.validate(strings.HasSuffix(base.Hostname(), "safaricom.co.ke")); err != nil {
		return nil, err
	}
	c.b2c = &cfg
	return c, nil
}

// B2C command ids: what the payment is, as M-Pesa records it.
const (
	BusinessPayment  = "BusinessPayment"  // the player's own money (a withdrawal)
	PromotionPayment = "PromotionPayment" // a prize
)

// B2CRequest is one payment to a customer.
type B2CRequest struct {
	// OriginatorConversationID is ours: unique per payout, it ties Daraja's result to the payout. Daraja refuses a
	// repeated one, which is a second guard against paying twice.
	OriginatorConversationID string
	MSISDN                   string
	AmountKES                int64
	CommandID                string
	Remarks                  string // 2 to 100 characters, shown on the statement
	Occasion                 string // echoed back in the result
}

// B2CAccepted identifies a payment Daraja queued; the outcome arrives later at the result URL.
type B2CAccepted struct {
	ConversationID string
}

var originatorRE = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// B2C sends money to a customer. Acceptance means only that Daraja queued the payment. An ErrRejected or
// ErrInvalidRequest means it was refused and no money moved; an ErrUnavailable means the outcome is unknown and the
// request must not be repeated, since it may have been paid.
func (c *Client) B2C(ctx context.Context, r B2CRequest) (B2CAccepted, error) {
	if c.b2c == nil || r.AmountKES <= 0 || r.AmountKES > MaxSTKAmountKES || !msisdnRE.MatchString(r.MSISDN) ||
		!originatorRE.MatchString(r.OriginatorConversationID) || (r.CommandID != BusinessPayment && r.CommandID != PromotionPayment) ||
		len(r.Remarks) < 2 || len(r.Remarks) > 100 || len(r.Occasion) > 100 {
		return B2CAccepted{}, ErrInvalidRequest
	}
	const path = "/mpesa/b2c/v3/paymentrequest"
	status, raw, err := c.call(ctx, path, map[string]any{
		"OriginatorConversationID": r.OriginatorConversationID, "InitiatorName": c.b2c.Initiator,
		"SecurityCredential": c.b2c.SecurityCredential, "CommandID": r.CommandID, "Amount": r.AmountKES,
		"PartyA": c.b2c.ShortCode, "PartyB": r.MSISDN, "Remarks": r.Remarks, "QueueTimeOutURL": c.b2c.TimeoutURL,
		"ResultURL": c.b2c.ResultURL, "Occassion": r.Occasion, // sic: Daraja's spelling
	})
	if err != nil {
		return B2CAccepted{}, err
	}
	if err := c.accepted(path, status, raw); err != nil {
		return B2CAccepted{}, err
	}
	var out struct {
		ConversationID string `json:"ConversationID"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ConversationID == "" {
		return B2CAccepted{}, fmt.Errorf("%w: b2c answer without a conversation id", ErrUnavailable)
	}
	return B2CAccepted{ConversationID: out.ConversationID}, nil
}

// B2CStatus asks Daraja whether a payout completed: by its receipt when a result named one, otherwise by our
// originator conversation id. occasion (the payout id) comes back in the answer, at the B2C status result URL.
func (c *Client) B2CStatus(ctx context.Context, receipt, originator, occasion string) (StatusRequest, error) {
	if c.b2c == nil || (receipt == "" && originator == "") || (receipt != "" && !receiptRE.MatchString(receipt)) ||
		(originator != "" && !originatorRE.MatchString(originator)) || len(occasion) > 100 {
		return StatusRequest{}, ErrInvalidRequest
	}
	const path = "/mpesa/transactionstatus/v1/query"
	status, raw, err := c.call(ctx, path, map[string]any{
		"Initiator": c.b2c.Initiator, "SecurityCredential": c.b2c.SecurityCredential,
		"CommandID": "TransactionStatusQuery", "TransactionID": receipt, "OriginalConversationID": originator,
		"PartyA": c.b2c.ShortCode, "IdentifierType": "4", "ResultURL": c.b2c.StatusResultURL,
		"QueueTimeOutURL": c.b2c.StatusTimeoutURL, "Remarks": "Payout check", "Occasion": occasion,
	})
	if err != nil {
		return StatusRequest{}, err
	}
	if err := c.accepted(path, status, raw); err != nil {
		return StatusRequest{}, err
	}
	var out struct {
		OriginatorConversationID string `json:"OriginatorConversationID"`
		ConversationID           string `json:"ConversationID"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.OriginatorConversationID == "" {
		return StatusRequest{}, fmt.Errorf("%w: status answer without a conversation id", ErrUnavailable)
	}
	return StatusRequest{OriginatorConversationID: out.OriginatorConversationID, ConversationID: out.ConversationID}, nil
}

// B2CResult is Daraja's report of a payout, posted to the result (or timeout) URL. Like every callback it is a claim.
type B2CResult struct {
	ResultCode               int32
	ResultDesc               string
	OriginatorConversationID string
	ConversationID           string
	// Set on success.
	Receipt   string
	AmountKES int64
	Receiver  string // "<msisdn> - <name>"
}

// Succeeded reports whether the result claims the payment went through.
func (r B2CResult) Succeeded() bool { return r.ResultCode == 0 }

// ParseB2CResult reads a B2C result body. A success must name the receipt and a positive whole amount.
func ParseB2CResult(body []byte) (B2CResult, error) {
	var env struct {
		Result struct {
			ResultCode               json.RawMessage `json:"ResultCode"`
			ResultDesc               string          `json:"ResultDesc"`
			OriginatorConversationID string          `json:"OriginatorConversationID"`
			ConversationID           string          `json:"ConversationID"`
			TransactionID            string          `json:"TransactionID"`
			Parameters               *struct {
				Items json.RawMessage `json:"ResultParameter"`
			} `json:"ResultParameters"`
		} `json:"Result"`
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return B2CResult{}, fmt.Errorf("%w: not json", ErrBadCallback)
	}
	in := env.Result
	code, ok := resultCode(in.ResultCode)
	if !ok || in.OriginatorConversationID == "" {
		return B2CResult{}, fmt.Errorf("%w: missing result code or conversation id", ErrBadCallback)
	}
	r := B2CResult{ResultCode: code, ResultDesc: in.ResultDesc, OriginatorConversationID: in.OriginatorConversationID, ConversationID: in.ConversationID}
	if in.Parameters != nil {
		for _, it := range keyValues(in.Parameters.Items) {
			str := strings.Trim(string(it.Value), `"`)
			switch it.Key {
			case "TransactionReceipt":
				r.Receipt = str
			case "TransactionAmount":
				if a, ok := wholeKES(it.Value); ok {
					r.AmountKES = a
				}
			case "ReceiverPartyPublicName":
				r.Receiver = str
			}
		}
	}
	if r.Receipt == "" && code == 0 && receiptRE.MatchString(in.TransactionID) {
		r.Receipt = in.TransactionID
	}
	if r.Succeeded() && (!receiptRE.MatchString(r.Receipt) || r.AmountKES == 0) {
		return B2CResult{}, fmt.Errorf("%w: success without receipt or amount", ErrBadCallback)
	}
	return r, nil
}

// CreditedMSISDN is the number a Transaction Status answer says received the money, or "" when it names none or masks
// it. For a payout the credit party is the customer.
func (r StatusResult) CreditedMSISDN() string {
	n, _, _ := strings.Cut(r.CreditParty, " ")
	if msisdnRE.MatchString(n) {
		return n
	}
	return ""
}
