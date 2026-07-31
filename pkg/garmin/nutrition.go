package garmin

import (
	"context"
	"encoding/json"
)

// NutritionService accesses nutrition-service endpoints.
type NutritionService struct{ c *Client }

// DailyFoodLog returns the day's food log.
func (s *NutritionService) DailyFoodLog(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/nutrition-service/food/logs/"+date.String(), nil, &raw)
	return raw, err
}

// DailyMeals returns the day's meals.
func (s *NutritionService) DailyMeals(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/nutrition-service/meals/"+date.String(), nil, &raw)
	return raw, err
}

// DailySettings returns the day's nutrition settings.
func (s *NutritionService) DailySettings(ctx context.Context, date Date) (json.RawMessage, error) {
	var raw json.RawMessage
	err := s.c.getJSON(ctx, "/nutrition-service/settings/"+date.String(), nil, &raw)
	return raw, err
}
