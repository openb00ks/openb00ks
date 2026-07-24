package domain

import "time"

type Role string

const (
	RoleAdmin      Role = "admin"
	RoleAccountant Role = "accountant"
	RoleUser       Role = "user"
)

type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}

type Entity struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

type EntityUser struct {
	UserID   string
	EntityID string
	Role     Role
}

type Account struct {
	ID        string
	EntityID  string
	Name      string
	Type      string
	CreatedAt time.Time
}

type Receipt struct {
	ID           string
	EntityID     string
	StorageKey   string
	ContentType  string
	SizeBytes    int64
	UploadedAt   time.Time
	AttachedAt   *time.Time
	OriginalName string
}

type Transaction struct {
	ID        string
	EntityID  string
	Date      time.Time
	Memo      string
	CreatedAt time.Time
}

type Entry struct {
	ID            string
	TransactionID string
	AccountID     string
	DebitCents    int64
	CreditCents   int64
}

type VendorRule struct {
	ID        string
	EntityID  string
	MatchType string
	Pattern   string
	AccountID string
	CreatedAt time.Time
}
