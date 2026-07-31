package garmin

import (
	"context"
	"encoding/json"
	"net/http"
)

// GraphQLService posts queries to the Garmin Connect GraphQL gateway.
type GraphQLService struct{ c *Client }

// GraphQLRequest is a GraphQL query envelope.
type GraphQLRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

// Query executes a GraphQL request and returns the raw response.
func (s *GraphQLService) Query(ctx context.Context, req GraphQLRequest) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.Do(ctx, http.MethodPost, "/graphql-gateway/graphql", nil, req, &raw)
	return raw, err
}
