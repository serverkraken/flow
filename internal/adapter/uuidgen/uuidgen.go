// Package uuidgen is the production ports.IDGen (UUIDv4).
package uuidgen

import "github.com/google/uuid"

type Gen struct{}

func (Gen) NewID() string { return uuid.NewString() }
