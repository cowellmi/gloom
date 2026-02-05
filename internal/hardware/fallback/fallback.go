package fallback

import (
	"errors"
	"time"
)

type Board struct{}

func NewBoard() *Board {
	return &Board{}
}

func (*Board) Now() (time.Time, error) {
	return time.Now(), nil
}

func (*Board) Sleep(d time.Duration) {
	time.Sleep(d)
}

func (*Board) ReadFile(name string) ([]byte, error) {
	return nil, errors.New("fallback: no storage available")
}
