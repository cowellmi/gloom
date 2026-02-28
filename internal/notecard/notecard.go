package notecard

import (
	"errors"
	"strings"

	"github.com/buger/jsonparser"
)

var (
	ResponseTimeout = errors.New("notecard: response timeout")
	NoteNotFound    = errors.New("notecard: note not found")
)

type I2CTxFn func(addr uint16, w, r []byte) error

type Client struct {
	UID string
	tx  I2CTxFn
}

func New(tx I2CTxFn) (*Client, error) {
	c := &Client{tx: tx}

	res, err := c.Do([]byte(`{"req":"card.version"}`))
	if err != nil {
		return nil, err
	}
	if uid, err := jsonparser.GetString(res, "device"); err == nil {
		c.UID = uid
	}

	if _, err := c.Do([]byte(`{"req":"hub.sync"}`)); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) Do(req []byte) ([]byte, error) {
	res, err := c.transaction(req)
	if err != nil {
		return nil, err
	}
	if e, err := jsonparser.GetString(res, "err"); err == nil && e != "" {
		if strings.Contains(e, "note-noexist") {
			return nil, NoteNotFound
		}
		return nil, errors.New(e)
	}
	return res, nil
}
