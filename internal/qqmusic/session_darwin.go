package qqmusic

import "context"

func LoadSession(ctx context.Context) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	return LoadMacSession()
}
