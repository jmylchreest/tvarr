package migrations

import (
	"gorm.io/gorm"
)

// migration010DynamicUserAgent updates all client detection rule expressions to use
// the unified @dynamic(request.headers):user-agent syntax instead of the static
// user_agent field which has been removed from RequestContextAccessor.
//
// This migration updates expressions that reference user_agent to use
// @dynamic(request.headers):user-agent for consistency with the unified
// dynamic field syntax introduced in migration 009.
func migration010DynamicUserAgent() Migration {
	return Migration{
		Version:     "010",
		Description: "Update client detection rules to use @dynamic() syntax for user-agent",
		Up: func(tx *gorm.DB) error {
			// Define the updates: map from old expression to new expression
			updates := []struct {
				name          string
				oldExpression string
				newExpression string
			}{
				{
					name:          "Android TV",
					oldExpression: `user_agent contains "Android" AND user_agent contains "TV"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"`,
				},
				{
					name:          "Samsung Smart TV",
					oldExpression: `user_agent contains "Tizen" OR user_agent contains "SMART-TV"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Tizen" OR @dynamic(request.headers):user-agent contains "SMART-TV"`,
				},
				{
					name:          "LG WebOS",
					oldExpression: `user_agent contains "webOS"`,
					newExpression: `@dynamic(request.headers):user-agent contains "webOS"`,
				},
				{
					name:          "Roku",
					oldExpression: `user_agent contains "Roku"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Roku"`,
				},
				{
					name:          "Apple TV",
					oldExpression: `user_agent contains "AppleTV" OR user_agent contains "tvOS"`,
					newExpression: `@dynamic(request.headers):user-agent contains "AppleTV" OR @dynamic(request.headers):user-agent contains "tvOS"`,
				},
				{
					name:          "iOS Safari",
					oldExpression: `user_agent contains "iPhone" OR user_agent contains "iPad"`,
					newExpression: `@dynamic(request.headers):user-agent contains "iPhone" OR @dynamic(request.headers):user-agent contains "iPad"`,
				},
				{
					name:          "Chrome Browser",
					oldExpression: `user_agent contains "Chrome" AND NOT user_agent contains "Edge"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Chrome" AND NOT @dynamic(request.headers):user-agent contains "Edge"`,
				},
				{
					name:          "Firefox Browser",
					oldExpression: `user_agent contains "Firefox"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Firefox"`,
				},
				{
					name:          "Safari Desktop",
					oldExpression: `user_agent contains "Safari" AND user_agent contains "Macintosh"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Safari" AND @dynamic(request.headers):user-agent contains "Macintosh"`,
				},
				{
					name:          "Edge Browser",
					oldExpression: `user_agent contains "Edge"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Edge"`,
				},
				{
					name:          "Android Mobile",
					oldExpression: `user_agent contains "Android" AND NOT user_agent contains "TV"`,
					newExpression: `@dynamic(request.headers):user-agent contains "Android" AND NOT @dynamic(request.headers):user-agent contains "TV"`,
				},
				{
					name:          "Generic Smart TV",
					oldExpression: `user_agent contains "SmartTV" OR user_agent contains "smart-tv"`,
					newExpression: `@dynamic(request.headers):user-agent contains "SmartTV" OR @dynamic(request.headers):user-agent contains "smart-tv"`,
				},
			}

			for _, u := range updates {
				// Update by name and old expression to be safe
				result := tx.Exec(`
					UPDATE client_detection_rules
					SET expression = ?
					WHERE name = ? AND expression = ?
				`, u.newExpression, u.name, u.oldExpression)
				if result.Error != nil {
					return result.Error
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Revert to old expression syntax
			updates := []struct {
				name          string
				oldExpression string
				newExpression string
			}{
				{
					name:          "Android TV",
					oldExpression: `@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"`,
					newExpression: `user_agent contains "Android" AND user_agent contains "TV"`,
				},
				{
					name:          "Samsung Smart TV",
					oldExpression: `@dynamic(request.headers):user-agent contains "Tizen" OR @dynamic(request.headers):user-agent contains "SMART-TV"`,
					newExpression: `user_agent contains "Tizen" OR user_agent contains "SMART-TV"`,
				},
				{
					name:          "LG WebOS",
					oldExpression: `@dynamic(request.headers):user-agent contains "webOS"`,
					newExpression: `user_agent contains "webOS"`,
				},
				{
					name:          "Roku",
					oldExpression: `@dynamic(request.headers):user-agent contains "Roku"`,
					newExpression: `user_agent contains "Roku"`,
				},
				{
					name:          "Apple TV",
					oldExpression: `@dynamic(request.headers):user-agent contains "AppleTV" OR @dynamic(request.headers):user-agent contains "tvOS"`,
					newExpression: `user_agent contains "AppleTV" OR user_agent contains "tvOS"`,
				},
				{
					name:          "iOS Safari",
					oldExpression: `@dynamic(request.headers):user-agent contains "iPhone" OR @dynamic(request.headers):user-agent contains "iPad"`,
					newExpression: `user_agent contains "iPhone" OR user_agent contains "iPad"`,
				},
				{
					name:          "Chrome Browser",
					oldExpression: `@dynamic(request.headers):user-agent contains "Chrome" AND NOT @dynamic(request.headers):user-agent contains "Edge"`,
					newExpression: `user_agent contains "Chrome" AND NOT user_agent contains "Edge"`,
				},
				{
					name:          "Firefox Browser",
					oldExpression: `@dynamic(request.headers):user-agent contains "Firefox"`,
					newExpression: `user_agent contains "Firefox"`,
				},
				{
					name:          "Safari Desktop",
					oldExpression: `@dynamic(request.headers):user-agent contains "Safari" AND @dynamic(request.headers):user-agent contains "Macintosh"`,
					newExpression: `user_agent contains "Safari" AND user_agent contains "Macintosh"`,
				},
				{
					name:          "Edge Browser",
					oldExpression: `@dynamic(request.headers):user-agent contains "Edge"`,
					newExpression: `user_agent contains "Edge"`,
				},
				{
					name:          "Android Mobile",
					oldExpression: `@dynamic(request.headers):user-agent contains "Android" AND NOT @dynamic(request.headers):user-agent contains "TV"`,
					newExpression: `user_agent contains "Android" AND NOT user_agent contains "TV"`,
				},
				{
					name:          "Generic Smart TV",
					oldExpression: `@dynamic(request.headers):user-agent contains "SmartTV" OR @dynamic(request.headers):user-agent contains "smart-tv"`,
					newExpression: `user_agent contains "SmartTV" OR user_agent contains "smart-tv"`,
				},
			}

			for _, u := range updates {
				result := tx.Exec(`
					UPDATE client_detection_rules
					SET expression = ?
					WHERE name = ? AND expression = ?
				`, u.newExpression, u.name, u.oldExpression)
				if result.Error != nil {
					return result.Error
				}
			}

			return nil
		},
	}
}
