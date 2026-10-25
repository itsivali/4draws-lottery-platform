package darajamock

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
)

// B2CFault shapes the next payout (and only the next one) and the Transaction Status answers about it. The zero value
// is a clean payout: accepted, paid, reported and confirmed.
type B2CFault struct {
	RequestHTTP     int   `json:"request_http"`      // refuse the request with this HTTP status: no money moves
	LostAnswer      bool  `json:"lost_answer"`       // pay, but answer the request with a 503 (the outcome looks unknown)
	ResultCode      int   `json:"result_code"`       // non-zero: the payout fails (no money moves) and the result says so
	ClaimResultCode *int  `json:"claim_result_code"` // the result claims this code whatever really happened (a lying result)
	DropResult      bool  `json:"drop_result"`       // never post the result
	DuplicateResult bool  `json:"duplicate_result"`  // post the result twice
	ResultTimeout   bool  `json:"result_timeout"`    // post to the timeout URL instead: the queue gave up, no money moves
	StatusHTTP      int   `json:"status_http"`       // answer status requests about it with this HTTP status
	StatusDrop      bool  `json:"status_drop"`       // accept status requests about it but never answer
	StatusAmountKES int64 `json:"status_amount_kes"` // the amount Transaction Status reports, when it differs
}

// Payout is a B2C payment the mock accepted.
type Payout struct {
	OriginatorConversationID string
	ConversationID           string
	MSISDN                   string
	AmountKES                int64
	CommandID                string
	Occasion                 string
	Receipt                  string // set when the money moved
	Paid                     bool
}

type b2cTxn struct {
	Payout
	fault                 B2CFault
	resultURL, timeoutURL string
}

var mockMSISDN = regexp.MustCompile(`^254[17][0-9]{8}$`)

// SetNextB2CFault makes the next payout behave as f.
func (s *Server) SetNextB2CFault(f B2CFault) {
	s.mu.Lock()
	s.nextB2C = f
	s.mu.Unlock()
}

// Payouts returns the payouts the mock accepted, in order.
func (s *Server) Payouts() []Payout {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Payout, 0, len(s.b2cOrder))
	for _, id := range s.b2cOrder {
		out = append(out, s.b2cTxns[id].Payout)
	}
	return out
}

func (s *Server) b2c(w http.ResponseWriter, _ *http.Request, body map[string]any) {
	if s.opts.Initiator == "" || body["InitiatorName"] != s.opts.Initiator || body["SecurityCredential"] != s.opts.SecurityCredential {
		writeErr(w, http.StatusBadRequest, "400.002.02", "Bad Request - Invalid Initiator")
		return
	}
	orig, _ := body["OriginatorConversationID"].(string)
	msisdn, _ := body["PartyB"].(string)
	cmd, _ := body["CommandID"].(string)
	amount, _ := body["Amount"].(float64)
	switch {
	case s.opts.B2CShortCode == "" || body["PartyA"] != s.opts.B2CShortCode:
		writeErr(w, http.StatusBadRequest, "400.002.02", "Bad Request - Invalid PartyA")
		return
	case cmd != "BusinessPayment" && cmd != "PromotionPayment" && cmd != "SalaryPayment":
		writeErr(w, http.StatusBadRequest, "400.002.02", "Bad Request - Invalid CommandID")
		return
	case amount < 10 || amount != float64(int64(amount)) || !mockMSISDN.MatchString(msisdn) || orig == "":
		writeErr(w, http.StatusBadRequest, "400.002.02", "Bad Request - Invalid request")
		return
	}
	s.mu.Lock()
	if _, dup := s.b2cTxns[orig]; dup {
		s.mu.Unlock()
		writeErr(w, http.StatusBadRequest, "400.002.05", "Duplicate OriginatorConversationID")
		return
	}
	f := s.nextB2C
	s.nextB2C = B2CFault{}
	if f.RequestHTTP != 0 {
		s.mu.Unlock()
		writeErr(w, f.RequestHTTP, "500.003.02", "Request refused")
		return
	}
	s.b2cSeq++
	occasion, _ := body["Occassion"].(string)
	t := &b2cTxn{fault: f, Payout: Payout{OriginatorConversationID: orig, ConversationID: "AG_B2C_" + strconv.Itoa(s.b2cSeq), MSISDN: msisdn,
		AmountKES: int64(amount), CommandID: cmd, Occasion: occasion}}
	t.resultURL, _ = body["ResultURL"].(string)
	t.timeoutURL, _ = body["QueueTimeOutURL"].(string)
	if f.ResultCode == 0 && !f.ResultTimeout {
		t.Paid, t.Receipt = true, "MBC"+strconv.Itoa(10_000_000 + s.b2cSeq)[1:]
	}
	s.b2cTxns[orig] = t
	s.b2cOrder = append(s.b2cOrder, orig)
	s.mu.Unlock()

	if f.LostAnswer {
		writeErr(w, http.StatusServiceUnavailable, "503.001.01", "Service unavailable")
	} else {
		writeJSON(w, http.StatusOK, map[string]string{"ConversationID": t.ConversationID, "OriginatorConversationID": orig,
			"ResponseCode": "0", "ResponseDescription": "Accept the service request successfully."})
	}
	if f.DropResult {
		return
	}
	out, url := b2cResultBody(t), t.resultURL
	if f.ResultTimeout {
		url = t.timeoutURL
	}
	n := 1
	if f.DuplicateResult {
		n = 2
	}
	s.async(func(ctx context.Context) {
		for range n {
			_ = s.post(ctx, url, out)
		}
	})
}

func b2cResultBody(t *b2cTxn) []byte {
	code := t.fault.ResultCode
	if t.fault.ResultTimeout {
		code = 1037
	}
	if t.fault.ClaimResultCode != nil {
		code = *t.fault.ClaimResultCode
	}
	result := map[string]any{"ResultType": 0, "ResultCode": code, "ResultDesc": resultDesc(code),
		"OriginatorConversationID": t.OriginatorConversationID, "ConversationID": t.ConversationID, "TransactionID": t.Receipt}
	if code == 0 {
		receipt := t.Receipt
		if receipt == "" { // a lying success for a payout that never paid
			receipt = "MBCFAKE000"
		}
		result["TransactionID"] = receipt
		result["ResultParameters"] = map[string]any{"ResultParameter": []map[string]any{
			{"Key": "TransactionAmount", "Value": t.AmountKES},
			{"Key": "TransactionReceipt", "Value": receipt},
			{"Key": "B2CRecipientIsRegisteredCustomer", "Value": "Y"},
			{"Key": "ReceiverPartyPublicName", "Value": t.MSISDN + " - John Doe"},
			{"Key": "B2CUtilityAccountAvailableFunds", "Value": 10116.00},
		}}
	}
	out, _ := json.Marshal(map[string]any{"Result": result})
	return out
}

// b2cStatus answers a Transaction Status request about a payout, found by receipt or by originator conversation id.
func (s *Server) b2cStatus(w http.ResponseWriter, body map[string]any) {
	receipt, _ := body["TransactionID"].(string)
	orig, _ := body["OriginalConversationID"].(string)
	resultURL, _ := body["ResultURL"].(string)
	occasion, _ := body["Occasion"].(string)
	s.mu.Lock()
	var t *b2cTxn
	if orig != "" {
		t = s.b2cTxns[orig]
	}
	for _, x := range s.b2cTxns {
		if t == nil && receipt != "" && x.Receipt == receipt {
			t = x
		}
	}
	var p Payout
	var f B2CFault
	if t != nil {
		p, f = t.Payout, t.fault
	}
	s.statusSeq++
	sOrig, conv := "mock-ts-"+strconv.Itoa(s.statusSeq), "AG_MOCK_"+strconv.Itoa(s.statusSeq)
	s.statusRequests = append(s.statusRequests, receipt+orig)
	s.mu.Unlock()

	if f.StatusHTTP != 0 {
		writeErr(w, f.StatusHTTP, "500.003.1001", "Service unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"OriginatorConversationID": sOrig, "ConversationID": conv,
		"ResponseCode": "0", "ResponseDescription": "Accept the service request successfully."})
	if f.StatusDrop {
		return
	}
	result := map[string]any{"ResultType": 0, "OriginatorConversationID": sOrig, "ConversationID": conv, "TransactionID": "MCKTS00000",
		"ReferenceData": map[string]any{"ReferenceItem": map[string]any{"Key": "Occasion", "Value": occasion}}}
	if t == nil || !p.Paid {
		result["ResultCode"], result["ResultDesc"] = 2001, "The transaction receipt number does not exist."
	} else {
		amount := p.AmountKES
		if f.StatusAmountKES != 0 {
			amount = f.StatusAmountKES
		}
		result["ResultCode"], result["ResultDesc"] = 0, "The service request is processed successfully."
		result["ResultParameters"] = map[string]any{"ResultParameter": []map[string]any{
			{"Key": "DebitPartyName", "Value": s.opts.B2CShortCode + " - Mock Org"},
			{"Key": "CreditPartyName", "Value": p.MSISDN + " - John Doe"},
			{"Key": "TransactionStatus", "Value": "Completed"},
			{"Key": "ReasonType", "Value": "Business Payment to Customer via API"},
			{"Key": "Amount", "Value": amount},
			{"Key": "ReceiptNo", "Value": p.Receipt},
		}}
	}
	out, err := json.Marshal(map[string]any{"Result": result})
	if err != nil {
		return
	}
	s.async(func(ctx context.Context) { _ = s.post(ctx, resultURL, out) })
}

func (s *Server) setB2CFault(w http.ResponseWriter, r *http.Request) {
	var f B2CFault
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&f); err != nil {
		http.Error(w, "bad fault", http.StatusBadRequest)
		return
	}
	s.SetNextB2CFault(f)
	w.WriteHeader(http.StatusNoContent)
}

// isPayout reports whether a Transaction Status request asks about a payout (by originator id or payout receipt).
func (s *Server) isPayout(body map[string]any) bool {
	receipt, _ := body["TransactionID"].(string)
	orig, _ := body["OriginalConversationID"].(string)
	s.mu.Lock()
	defer s.mu.Unlock()
	if orig != "" {
		return true
	}
	for _, t := range s.b2cTxns {
		if receipt != "" && t.Receipt == receipt {
			return true
		}
	}
	return false
}
