// Copyright 2018 The Kubernetes Authors. Licensed under Apache-2.0.
package host
import("context";"gofx/rt";"golang.org/x/oauth2";"github.com/reoring/gofx/ports/client-go/host/sync")
func TokenExpiry(v any)any{return effect(func(ctx context.Context)(any,error){token:=value[*oauth2.Token](ctx,rt.SomeOf(v));if token.Expiry.IsZero(){return rt.NoneOf(),nil};return rt.SomeOf(hostsync.InstantValue(token.Expiry)),nil})}
func MakeToken(access,kind,refresh,expiry any)any{return effect(func(context.Context)(any,error){token:=&oauth2.Token{AccessToken:access.(string),TokenType:kind.(string),RefreshToken:refresh.(string)};if instant,ok:=rt.OptionValue(expiry);ok{token.Expiry=hostsync.TimeValue(instant)};v,_:=rt.OptionValue(external(token));return v,nil})}
func LegacyOAuthCancelRequest(v any)any{return effect(func(ctx context.Context)(any,error){transport:=oauth2.Transport{};transport.CancelRequest(request(ctx,v));return rt.Unit{},nil})}
