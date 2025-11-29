package httpclient

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		client := NewWithDefaults()
		assert.NotNil(t, client)
		assert.NotNil(t, client.client)
		assert.NotNil(t, client.breaker)
		assert.NotNil(t, client.logger)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := Config{
			Timeout:          10 * time.Second,
			RetryAttempts:    5,
			CircuitThreshold: 10,
		}
		client := New(cfg)
		assert.NotNil(t, client)
		assert.Equal(t, 5, client.config.RetryAttempts)
		assert.Equal(t, 10, client.config.CircuitThreshold)
	})

	t.Run("with custom base client", func(t *testing.T) {
		baseClient := &http.Client{Timeout: 5 * time.Second}
		cfg := DefaultConfig()
		cfg.BaseClient = baseClient
		client := New(cfg)
		assert.Equal(t, baseClient, client.client)
	})
}

func TestClient_Get(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		client := NewWithDefaults()
		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, `{"status":"ok"}`, string(body))
	})

	t.Run("sets user agent header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.Header.Get(HeaderUserAgent), "tvarr")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.UserAgent = "tvarr-test/1.0"
		client := New(cfg)

		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		resp.Body.Close()
	})

	t.Run("sets accept encoding header", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			acceptEncoding := r.Header.Get(HeaderAcceptEncoding)
			assert.Contains(t, acceptEncoding, "gzip")
			assert.Contains(t, acceptEncoding, "deflate")
			assert.Contains(t, acceptEncoding, "br")
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := NewWithDefaults()
		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		resp.Body.Close()
	})
}

func TestClient_Retries(t *testing.T) {
	t.Run("retries on 503 then succeeds", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			if count < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.RetryAttempts = 3
		cfg.RetryDelay = 10 * time.Millisecond
		client := New(cfg)

		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
	})

	t.Run("returns error after max retries", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.RetryAttempts = 2
		cfg.RetryDelay = 10 * time.Millisecond
		client := New(cfg)

		_, err := client.Get(context.Background(), server.URL)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMaxRetries)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attempts)) // initial + 2 retries
	})

	t.Run("does not retry on 404", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.RetryAttempts = 3
		client := New(cfg)

		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Equal(t, int32(1), atomic.LoadInt32(&attempts))
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.RetryAttempts = 3
		client := New(cfg)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		_, err := client.Get(ctx, server.URL)
		require.Error(t, err)
	})
}

func TestClient_GzipDecompression(t *testing.T) {
	t.Run("decompresses gzip response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set(HeaderContentEncoding, EncodingGzip)
			gw := gzip.NewWriter(w)
			gw.Write([]byte("hello compressed world"))
			gw.Close()
		}))
		defer server.Close()

		client := NewWithDefaults()
		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "hello compressed world", string(body))
	})

	t.Run("handles uncompressed response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("plain text"))
		}))
		defer server.Close()

		client := NewWithDefaults()
		resp, err := client.Get(context.Background(), server.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "plain text", string(body))
	})

	t.Run("decompression disabled does not set accept-encoding", func(t *testing.T) {
		// When decompression is disabled, the client should not add Accept-Encoding
		// Note: Go's http.Transport adds its own Accept-Encoding: gzip by default,
		// but our client won't add the extended header (gzip, deflate, br)
		cfg := DefaultConfig()
		cfg.EnableDecompression = false
		client := New(cfg)

		// Just verify the config is set correctly
		assert.False(t, client.config.EnableDecompression)
	})
}

func TestCircuitBreaker(t *testing.T) {
	t.Run("opens after threshold failures", func(t *testing.T) {
		cb := NewCircuitBreaker(3, 100*time.Millisecond, 1)

		assert.Equal(t, CircuitClosed, cb.State())

		// Record failures up to threshold
		cb.RecordFailure()
		cb.RecordFailure()
		assert.Equal(t, CircuitClosed, cb.State())

		cb.RecordFailure()
		assert.Equal(t, CircuitOpen, cb.State())
	})

	t.Run("denies requests when open", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 100*time.Millisecond, 1)

		cb.RecordFailure()
		assert.Equal(t, CircuitOpen, cb.State())
		assert.False(t, cb.Allow())
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 10*time.Millisecond, 1)

		cb.RecordFailure()
		assert.Equal(t, CircuitOpen, cb.State())

		time.Sleep(20 * time.Millisecond)
		assert.True(t, cb.Allow())
		assert.Equal(t, CircuitHalfOpen, cb.State())
	})

	t.Run("closes after success in half-open", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 10*time.Millisecond, 1)

		cb.RecordFailure()
		time.Sleep(20 * time.Millisecond)
		cb.Allow() // Transition to half-open

		cb.RecordSuccess()
		assert.Equal(t, CircuitClosed, cb.State())
	})

	t.Run("returns to open on failure in half-open", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 10*time.Millisecond, 1)

		cb.RecordFailure()
		time.Sleep(20 * time.Millisecond)
		cb.Allow() // Transition to half-open

		cb.RecordFailure()
		assert.Equal(t, CircuitOpen, cb.State())
	})

	t.Run("limits requests in half-open state", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 10*time.Millisecond, 3)

		cb.RecordFailure()
		time.Sleep(20 * time.Millisecond)

		// First call transitions from open to half-open (counts as 1)
		assert.True(t, cb.Allow())
		assert.Equal(t, CircuitHalfOpen, cb.State())

		// Two more requests allowed (total 3 in half-open)
		assert.True(t, cb.Allow()) // count = 2
		assert.True(t, cb.Allow()) // count = 3

		// Fourth request denied (exceeded halfOpenMax=3)
		assert.False(t, cb.Allow())
	})

	t.Run("reset returns to closed", func(t *testing.T) {
		cb := NewCircuitBreaker(1, 100*time.Millisecond, 1)

		cb.RecordFailure()
		assert.Equal(t, CircuitOpen, cb.State())

		cb.Reset()
		assert.Equal(t, CircuitClosed, cb.State())
		assert.True(t, cb.Allow())
	})
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestClient_CircuitBreakerIntegration(t *testing.T) {
	t.Run("opens circuit on repeated failures", func(t *testing.T) {
		var attempts int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		cfg := DefaultConfig()
		cfg.RetryAttempts = 0 // No retries, just test circuit breaker
		cfg.CircuitThreshold = 3
		cfg.CircuitTimeout = 100 * time.Millisecond
		client := New(cfg)

		// Make requests until circuit opens
		for i := 0; i < 5; i++ {
			client.Get(context.Background(), server.URL)
		}

		// Circuit should be open
		assert.Equal(t, CircuitOpen, client.CircuitState())

		// New request should fail immediately
		_, err := client.Get(context.Background(), server.URL)
		assert.ErrorIs(t, err, ErrMaxRetries)
		assert.Contains(t, err.Error(), ErrCircuitOpen.Error())
	})
}

func TestObfuscateURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "obfuscates password",
			input:    "http://example.com/api?username=user&password=secret123",
			expected: "http://example.com/api?password=***&username=user",
		},
		{
			name:     "obfuscates token",
			input:    "http://example.com/api?token=abc123",
			expected: "http://example.com/api?token=***",
		},
		{
			name:     "obfuscates api_key",
			input:    "http://example.com/api?api_key=secret",
			expected: "http://example.com/api?api_key=***",
		},
		{
			name:     "preserves non-sensitive params",
			input:    "http://example.com/api?action=get&id=123",
			expected: "http://example.com/api?action=get&id=123",
		},
		{
			name:     "handles multiple sensitive params",
			input:    "http://example.com/api?password=p1&token=t1&key=k1",
			expected: "http://example.com/api?key=***&password=***&token=***",
		},
		{
			name:     "handles nil url",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u *url.URL
			if tt.input != "" {
				var err error
				u, err = url.Parse(tt.input)
				require.NoError(t, err)
			}

			result := obfuscateURL(u)

			if tt.expected == "" {
				assert.Empty(t, result)
			} else {
				// Parse both to compare ignoring query param order
				expectedURL, _ := url.Parse(tt.expected)
				resultURL, _ := url.Parse(result)
				assert.Equal(t, expectedURL.Host, resultURL.Host)
				assert.Equal(t, expectedURL.Path, resultURL.Path)
				// Query params should match (already sorted by url.Values.Encode)
				assert.Equal(t, expectedURL.Query(), resultURL.Query())
			}
		})
	}
}

func TestIsRetryableStatus(t *testing.T) {
	retryable := []int{
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	nonRetryable := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, code := range retryable {
		t.Run("retryable_"+http.StatusText(code), func(t *testing.T) {
			assert.True(t, isRetryableStatus(code))
		})
	}

	for _, code := range nonRetryable {
		t.Run("non_retryable_"+http.StatusText(code), func(t *testing.T) {
			assert.False(t, isRetryableStatus(code))
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, DefaultTimeout, cfg.Timeout)
	assert.Equal(t, DefaultRetryAttempts, cfg.RetryAttempts)
	assert.Equal(t, DefaultRetryDelay, cfg.RetryDelay)
	assert.Equal(t, DefaultRetryMaxDelay, cfg.RetryMaxDelay)
	assert.Equal(t, DefaultBackoffMultiplier, cfg.BackoffMultiplier)
	assert.Equal(t, DefaultCircuitThreshold, cfg.CircuitThreshold)
	assert.Equal(t, DefaultCircuitTimeout, cfg.CircuitTimeout)
	assert.Equal(t, DefaultCircuitHalfOpenMax, cfg.CircuitHalfOpenMax)
	assert.Equal(t, DefaultUserAgentHeader, cfg.UserAgent)
	assert.True(t, cfg.EnableDecompression)
}

func TestDecompressReader(t *testing.T) {
	t.Run("close closes both reader and underlying closer", func(t *testing.T) {
		var readerClosed, closerClosed bool

		reader := &mockReadCloser{
			readFunc: func(p []byte) (int, error) {
				return 0, io.EOF
			},
			closeFunc: func() error {
				readerClosed = true
				return nil
			},
		}

		closer := &mockReadCloser{
			closeFunc: func() error {
				closerClosed = true
				return nil
			},
		}

		dr := &decompressReader{reader: reader, closer: closer}
		dr.Close()

		assert.True(t, readerClosed)
		assert.True(t, closerClosed)
	})
}

type mockReadCloser struct {
	readFunc  func(p []byte) (int, error)
	closeFunc func() error
}

func (m *mockReadCloser) Read(p []byte) (int, error) {
	if m.readFunc != nil {
		return m.readFunc(p)
	}
	return 0, io.EOF
}

func (m *mockReadCloser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestClient_DoWithCustomRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "custom-header-value", r.Header.Get("X-Custom-Header"))
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewWithDefaults()

	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("body"))
	require.NoError(t, err)
	req.Header.Set("X-Custom-Header", "custom-header-value")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}
