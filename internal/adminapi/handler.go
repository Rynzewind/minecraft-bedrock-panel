package adminapi

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrFrameTooLarge = errors.New("admin frame exceeds 1 MiB")

type Controller interface {
	Handle(context.Context, string, json.RawMessage) (any, error)
}
type CodedError interface {
	error
	Code() string
}

func ServeConnection(ctx context.Context, rw io.ReadWriter, c Controller) error {
	r := bufio.NewReader(rw)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		var n uint32
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return err
		}
		if n > MaxFrameSize {
			return ErrFrameTooLarge
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return err
		}
		var req Request
		if err := json.Unmarshal(b, &req); err != nil {
			if err = writeResponse(rw, Response{Error: &APIError{Code: "invalid_json", Message: "Invalid administration request."}}); err != nil {
				return err
			}
			continue
		}
		result, err := c.Handle(ctx, req.Method, req.Params)
		resp := Response{ID: req.ID, Result: result}
		if err != nil {
			code := "request_failed"
			var ce CodedError
			if errors.As(err, &ce) {
				code = ce.Code()
			}
			resp.Result = nil
			resp.Error = &APIError{Code: code, Message: safeMessage(code, err)}
		}
		if err = writeResponse(rw, resp); err != nil {
			return err
		}
	}
}
func safeMessage(code string, cause error) string {
	switch code {
	case "player_active":
		return "The player is currently connected."
	case "account_assigned":
		return "That account is assigned to another player."
	case "pool_exhausted":
		return "No proxy player account is available."
	case "not_found":
		return "The requested item was not found."
	case "invalid_request":
		return "The request is invalid."
	default:
		if cause == nil {
			return "The operation could not be completed."
		}
		message := cause.Error()
		lower := strings.ToLower(message)
		for _, secretMarker := range []string{"access_token", "refresh_token", "authorization: bearer", `"token":"`} {
			if strings.Contains(lower, secretMarker) {
				return "The operation failed, but its details contained protected authentication data."
			}
		}
		return message
	}
}
func writeResponse(w io.Writer, v Response) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxFrameSize {
		return fmt.Errorf("encode response: %w", ErrFrameTooLarge)
	}
	if err = binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
func ReadResponse(r io.Reader) (Response, error) {
	var n uint32
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return Response{}, err
	}
	if n > MaxFrameSize {
		return Response{}, ErrFrameTooLarge
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return Response{}, err
	}
	var resp Response
	err := json.Unmarshal(b, &resp)
	return resp, err
}
func WriteRequest(w io.Writer, v Request) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	if err = binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
