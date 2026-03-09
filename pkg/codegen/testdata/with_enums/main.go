package with_enums

import "context"

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

type router struct{}

func (router) Query() {}

var gotrpc router

type Router struct{}

var r = &Router{}

func setup() {
	gotrpc.Query(r, "getStatus", func(ctx context.Context, input struct{}) (Status, error) {
		return StatusActive, nil
	})
}
