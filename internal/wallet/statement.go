package wallet

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/itsivali/4draws-lottery-platform/internal/wallet/store"
)

// ErrBadCursor is returned for a page cursor that is not a transaction id.
var ErrBadCursor = errors.New("wallet: bad cursor")

// Reader answers players' questions about their own wallet. It only reads.
type Reader struct {
	pool *pgxpool.Pool
	svc  *Service
}

// NewReader returns a Reader over pool.
func NewReader(pool *pgxpool.Pool) *Reader { return &Reader{pool: pool, svc: &Service{}} }

// Transaction is one movement of a wallet: credits positive, debits negative.
type Transaction struct {
	ID            string
	Kind          string
	AmountKES     int64
	ReferenceType *string
	ReferenceID   *string
	CreatedAt     time.Time
}

// Statement is a wallet's balance, debt and a page of its transactions, newest first, read in one snapshot.
type Statement struct {
	BalanceKES   int64
	DebtKES      int64
	Transactions []Transaction
	// NextBefore continues the list (pass it as before); empty on the last page.
	NextBefore string
}

// Statement reads the player's wallet. before is the last transaction id of the previous page, or "".
func (r *Reader) Statement(ctx context.Context, user string, limit int, before string) (Statement, error) {
	var cursor *string
	if before != "" {
		cursor = &before
	}
	var st Statement
	err := pgx.BeginTxFunc(ctx, r.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var err error
		if st.BalanceKES, err = r.svc.Balance(ctx, tx, user); err != nil {
			return err
		}
		if st.DebtKES, err = r.svc.Debt(ctx, tx, user); err != nil {
			return err
		}
		// one extra row tells whether another page exists
		rows, err := store.New(tx).WalletTransactions(ctx, store.WalletTransactionsParams{UserID: user, Before: cursor, Lim: int32(limit) + 1}) //nolint:gosec // callers pass small limits
		if err != nil {
			return err
		}
		more := len(rows) > limit
		if more {
			rows = rows[:limit]
		}
		st.Transactions = make([]Transaction, 0, len(rows))
		for _, w := range rows {
			st.Transactions = append(st.Transactions, Transaction{ID: w.ID, Kind: w.Kind, AmountKES: w.AmountKes,
				ReferenceType: w.ReferenceType, ReferenceID: w.ReferenceID, CreatedAt: w.CreatedAt.Time})
		}
		if more {
			st.NextBefore = rows[len(rows)-1].ID
		}
		return nil
	})
	if pe := (*pgconn.PgError)(nil); errors.As(err, &pe) && pe.Code == "22P02" { // not a uuid
		return Statement{}, ErrBadCursor
	}
	return st, err
}
