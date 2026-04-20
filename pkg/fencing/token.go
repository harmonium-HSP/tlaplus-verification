package fencing

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-redis/redis/v8"
)

type Token int64

type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
	Incr(ctx context.Context, key string) (int64, error)
}

var ErrStaleToken = errors.New("stale token rejected")

func NewToken(value int64) Token {
	return Token(value)
}

func (t Token) String() string {
	return fmt.Sprintf("%d", t)
}

func (t Token) Value() int64 {
	return int64(t)
}

type Writer struct {
	store  Store
	prefix string
}

func NewWriter(store Store) *Writer {
	return &Writer{
		store:  store,
		prefix: "fencing:",
	}
}

func (w *Writer) Write(ctx context.Context, key string, data string, token Token) error {
	fenceKey := w.prefix + key + ":fence"
	dataKey := w.prefix + key + ":data"

	currentVal, err := w.store.Get(ctx, fenceKey)
	if err != nil && err != redis.Nil {
		return err
	}

	var currentToken Token = 0
	if currentVal != "" {
		fmt.Sscanf(currentVal, "%d", &currentToken)
	}

	if token <= currentToken {
		return ErrStaleToken
	}

	if err := w.store.Set(ctx, fenceKey, token.String()); err != nil {
		return err
	}

	return w.store.Set(ctx, dataKey, data)
}

func (w *Writer) Read(ctx context.Context, key string) (string, Token, error) {
	fenceKey := w.prefix + key + ":fence"
	dataKey := w.prefix + key + ":data"

	fenceVal, err := w.store.Get(ctx, fenceKey)
	if err != nil {
		return "", 0, err
	}

	dataVal, err := w.store.Get(ctx, dataKey)
	if err != nil {
		return "", 0, err
	}

	var token Token
	if _, err := fmt.Sscanf(fenceVal, "%d", &token); err != nil {
		return "", 0, err
	}

	return dataVal, token, nil
}
