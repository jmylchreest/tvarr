// Package xtream provides a Go client for the Xtream Codes API.
//
// Xtream Codes is an IPTV panel system that exposes a REST API for accessing
// live TV streams, video on demand (VOD), TV series, and EPG (Electronic
// Program Guide) data.
//
// # Basic Usage
//
//	client := xtream.NewClient("http://example.com:8080", "username", "password")
//
//	// Get server and user info
//	info, err := client.GetAuthInfo(ctx)
//
//	// List live stream categories
//	categories, err := client.GetLiveCategories(ctx)
//
//	// List all live streams
//	streams, err := client.GetLiveStreams(ctx, nil)
//
//	// List streams in a specific category
//	streams, err := client.GetLiveStreams(ctx, &xtream.StreamsOptions{CategoryID: "1"})
//
//	// Get EPG for a stream
//	epg, err := client.GetShortEPG(ctx, 12345, 10)
//
// # Stream URLs
//
// The client can generate properly formatted stream URLs:
//
//	// Live stream URL
//	url := client.GetLiveStreamURL(12345, "ts")
//
//	// VOD stream URL
//	url := client.GetVODStreamURL(67890, "mp4")
//
//	// Series episode URL
//	url := client.GetSeriesStreamURL(11111, "mkv")
//
// # API Reference
//
// This client is based on the Xtream Codes API documentation and implementations:
//   - https://github.com/tellytv/go.xtream-codes
//   - https://github.com/sherif-fanous/xtreamcodes
//
// # API Endpoints
//
// The Xtream Codes API uses the following endpoint pattern:
//
//	{baseURL}/player_api.php?username={user}&password={pass}&action={action}
//
// Available actions:
//   - (no action): Get server info and authentication status
//   - get_live_categories: List live stream categories
//   - get_vod_categories: List VOD categories
//   - get_series_categories: List series categories
//   - get_live_streams: List live streams (optional: category_id)
//   - get_vod_streams: List VOD content (optional: category_id)
//   - get_series: List series (optional: category_id)
//   - get_series_info: Get series details (required: series_id)
//   - get_vod_info: Get VOD details (required: vod_id)
//   - get_short_epg: Get short EPG (required: stream_id, optional: limit)
//   - get_simple_data_table: Get full EPG (required: stream_id)
//
// Additional endpoints:
//   - {baseURL}/xmltv.php?username={user}&password={pass}: Full XMLTV EPG
//   - {baseURL}/live/{user}/{pass}/{streamID}.{ext}: Live stream
//   - {baseURL}/movie/{user}/{pass}/{vodID}.{ext}: VOD stream
//   - {baseURL}/series/{user}/{pass}/{episodeID}.{ext}: Series episode
package xtream
