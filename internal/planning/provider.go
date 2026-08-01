package planning

import "context"

type Provider interface {
	Clarify(context.Context, Session) ([]Question, error)
	BuildPlan(context.Context, Session) (Plan, error)
	Name() string
}

type RoleReviewProvider interface {
	ReviewRole(context.Context, string, Session) (RoleReview, error)
	Coordinate(context.Context, Session, []RoleReview) (Coordination, error)
	ReviewName() string
}
