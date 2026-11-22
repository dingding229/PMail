package models

// RegistrationGuard provides a singleton database write lock for first-user
// creation, independent of the proposed account name and server process.
type RegistrationGuard struct {
	ID         int   `xorm:"id pk notnull"`
	Generation int64 `xorm:"generation bigint notnull default(0)"`
}

func (RegistrationGuard) TableName() string { return "registration_guard" }
