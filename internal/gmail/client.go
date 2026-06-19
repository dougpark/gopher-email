package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	gmailsvc "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// Client wraps the Gmail API service.
type Client struct {
	svc    *gmailsvc.Service
	userID string
}

// New creates a Gmail API client from an authenticated HTTP client.
func New(ctx context.Context, httpClient *http.Client) (*Client, error) {
	svc, err := gmailsvc.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("creating gmail service: %w", err)
	}
	return &Client{svc: svc, userID: "me"}, nil
}

// ListByLabel returns all message IDs that currently carry the given label name.
// It pages through all results automatically.
func (c *Client) ListByLabel(ctx context.Context, labelName string) ([]string, error) {
	var ids []string

	call := c.svc.Users.Messages.List(c.userID).Q("label:" + labelName).Context(ctx)
	err := withRetry(func() error {
		return call.Pages(ctx, func(page *gmailsvc.ListMessagesResponse) error {
			for _, m := range page.Messages {
				ids = append(ids, m.Id)
			}
			return nil
		})
	})
	if err != nil {
		return nil, fmt.Errorf("listing messages with label %q: %w", labelName, err)
	}
	return ids, nil
}

// GetRaw fetches a message in RFC 2822 format and returns the decoded bytes.
func (c *Client) GetRaw(ctx context.Context, msgID string) ([]byte, error) {
	var raw string

	err := withRetry(func() error {
		msg, err := c.svc.Users.Messages.Get(c.userID, msgID).Format("raw").Context(ctx).Do()
		if err != nil {
			return err
		}
		raw = msg.Raw
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fetching message %q: %w", msgID, err)
	}

	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		// Gmail sometimes uses standard encoding; try both.
		decoded, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("decoding base64 for message %q: %w", msgID, err)
		}
	}
	return decoded, nil
}

// BatchModify removes removeLabels and adds addLabels to all provided message IDs.
func (c *Client) BatchModify(ctx context.Context, ids, removeLabels, addLabels []string) error {
	// Resolve human-readable label names to label IDs.
	removeLabelIDs, err := c.resolveLabelIDs(ctx, removeLabels)
	if err != nil {
		return err
	}
	addLabelIDs, err := c.resolveLabelIDs(ctx, addLabels)
	if err != nil {
		return err
	}

	req := &gmailsvc.BatchModifyMessagesRequest{
		Ids:            ids,
		RemoveLabelIds: removeLabelIDs,
		AddLabelIds:    addLabelIDs,
	}

	return withRetry(func() error {
		return c.svc.Users.Messages.BatchModify(c.userID, req).Context(ctx).Do()
	})
}

// resolveLabelIDs converts label display names (e.g. "gSave") to Gmail label IDs.
func (c *Client) resolveLabelIDs(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	var list *gmailsvc.ListLabelsResponse
	err := withRetry(func() error {
		var err error
		list, err = c.svc.Users.Labels.List(c.userID).Context(ctx).Do()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("listing labels: %w", err)
	}

	nameToID := make(map[string]string, len(list.Labels))
	for _, l := range list.Labels {
		nameToID[l.Name] = l.Id
	}

	var ids []string
	for _, n := range names {
		id, ok := nameToID[n]
		if !ok {
			return nil, fmt.Errorf("label %q not found in Gmail account", n)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// withRetry wraps an operation with exponential backoff (max 5 retries, 32s cap).
func withRetry(op func() error) error {
	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 2 * time.Minute
	bo.MaxInterval = 32 * time.Second
	return backoff.Retry(op, bo)
}
