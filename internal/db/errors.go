package db

import "errors"

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrReceiptAlreadyAttached = errors.New("receipt already attached")
var ErrAccountEntityMismatch = errors.New("account entity mismatch")
var ErrAccountInUse = errors.New("account in use")
var ErrReceiptEntityMismatch = errors.New("receipt entity mismatch")
