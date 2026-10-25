package identity

import "context"

type RequestIdentity struct {
	ActorID   string
	Principal *VerifiedPrincipal
}

type requestIdentityKey struct{}

func WithRequestIdentity(ctx context.Context, actor RequestIdentity) context.Context {
	if actor.Principal != nil {
		copy := *actor.Principal
		actor.Principal = &copy
	}
	return context.WithValue(ctx, requestIdentityKey{}, actor)
}

func RequestIdentityFrom(ctx context.Context) (RequestIdentity, bool) {
	actor, ok := ctx.Value(requestIdentityKey{}).(RequestIdentity)
	return actor, ok && actor.ActorID != ""
}
