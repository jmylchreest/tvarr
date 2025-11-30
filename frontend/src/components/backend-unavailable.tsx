'use client';

import { useState, useEffect } from 'react';
import { getBackendUrl } from '@/lib/config';

interface BackendUnavailableProps {
  onRetry: () => void;
  isRetrying?: boolean;
  backendUrl?: string;
}

export function BackendUnavailable({
  onRetry,
  isRetrying = false,
  backendUrl,
}: BackendUnavailableProps) {
  const [retryCount, setRetryCount] = useState(0);

  const handleRetry = () => {
    setRetryCount((prev) => prev + 1);
    onRetry();
  };

  useEffect(() => {
    // Auto-retry every 30 seconds for the first 5 attempts
    if (retryCount < 5) {
      const timer = setTimeout(() => {
        handleRetry();
      }, 30000);

      return () => clearTimeout(timer);
    }
  }, [retryCount]);

  const displayUrl = backendUrl || getBackendUrl();

  return (
    <>
      <style jsx>{`
        .error-page {
          min-height: 100vh;
          background: oklch(0.13 0.0047 264.29);
          color: oklch(0.8 0.02 240.1);
          font-family: ui-sans-serif, system-ui, sans-serif;
          display: flex;
          align-items: center;
          justify-content: center;
          padding: 1rem;
        }
        .error-container {
          width: 100%;
          max-width: 48rem;
          display: flex;
          flex-direction: column;
          gap: 1.5rem;
        }
        .error-card {
          background: oklch(0.15 0.008 240.1);
          border: 1px solid oklch(0.98 0.003 265.75 / 0.2);
          border-radius: 0.5rem;
          overflow: hidden;
        }
        .error-card-destructive {
          border-color: oklch(0.73 0.127 15.27 / 0.5);
        }
        .error-header {
          padding: 1.5rem 1.5rem 1rem;
          text-align: center;
        }
        .error-icon {
          width: 4rem;
          height: 4rem;
          margin: 0 auto 1rem;
          background: oklch(0.73 0.127 15.27 / 0.1);
          border-radius: 50%;
          display: flex;
          align-items: center;
          justify-content: center;
        }
        .error-title {
          font-size: 1.5rem;
          font-weight: 700;
          color: oklch(0.73 0.127 15.27);
          margin-bottom: 0.5rem;
        }
        .error-description {
          font-size: 1rem;
          color: oklch(0.6 0.02 240.1);
          line-height: 1.5;
        }
        .error-content {
          padding: 0 1.5rem 1.5rem;
        }
        .details-box {
          background: oklch(0.12 0.008 240.1);
          border-radius: 0.5rem;
          padding: 1rem;
          margin-bottom: 1.5rem;
        }
        .details-item {
          display: flex;
          align-items: center;
          gap: 0.5rem;
          margin-bottom: 0.75rem;
          font-size: 0.875rem;
          font-weight: 500;
        }
        .url-box {
          background: oklch(0.13 0.0047 264.29);
          border: 1px solid oklch(0.18 0.008 240.1);
          border-radius: 0.375rem;
          padding: 0.75rem;
          font-family: ui-monospace, SFMono-Regular, monospace;
          font-size: 0.875rem;
          word-break: break-all;
          margin-bottom: 0.75rem;
        }
        .status-badge {
          background: oklch(0.73 0.127 15.27);
          color: oklch(0.98 0.006 264.29);
          padding: 0.125rem 0.5rem;
          border-radius: 0.375rem;
          font-size: 0.75rem;
          font-weight: 500;
        }
        .retry-section {
          display: flex;
          flex-direction: column;
          gap: 0.75rem;
          align-items: center;
          justify-content: center;
        }
        @media (min-width: 640px) {
          .retry-section {
            flex-direction: row;
          }
        }
        .retry-button {
          background: oklch(0.6 0.12 240.1);
          color: oklch(0.98 0.006 264.29);
          border: none;
          padding: 0.625rem 1.25rem;
          border-radius: 0.375rem;
          font-weight: 500;
          cursor: pointer;
          display: flex;
          align-items: center;
          gap: 0.5rem;
          width: 100%;
          justify-content: center;
          transition: background-color 0.2s;
        }
        @media (min-width: 640px) {
          .retry-button {
            width: auto;
          }
        }
        .retry-button:hover:not(:disabled) {
          background: oklch(0.55 0.12 240.1);
        }
        .retry-button:disabled {
          opacity: 0.5;
          cursor: not-allowed;
        }
        .retry-info {
          font-size: 0.875rem;
          color: oklch(0.6 0.02 240.1);
        }
        .troubleshooting-title {
          font-size: 1.125rem;
          font-weight: 600;
          margin-bottom: 1rem;
          display: flex;
          align-items: center;
          gap: 0.5rem;
        }
        .troubleshooting-content {
          font-size: 0.875rem;
          line-height: 1.5;
        }
        .troubleshooting-section {
          margin-bottom: 1rem;
        }
        .troubleshooting-heading {
          font-weight: 500;
          margin-bottom: 0.25rem;
        }
        .troubleshooting-list {
          list-style: disc;
          list-style-position: inside;
          color: oklch(0.6 0.02 240.1);
          margin-left: 1rem;
        }
        .troubleshooting-list li {
          margin-bottom: 0.25rem;
        }
        .code {
          background: oklch(0.12 0.008 240.1);
          padding: 0.125rem 0.25rem;
          border-radius: 0.25rem;
          font-family: ui-monospace, SFMono-Regular, monospace;
        }
        .test-button {
          background: transparent;
          border: 1px solid oklch(0.18 0.008 240.1);
          color: oklch(0.8 0.02 240.1);
          padding: 0.5rem 1rem;
          border-radius: 0.375rem;
          font-size: 0.875rem;
          cursor: pointer;
          text-decoration: none;
          display: inline-flex;
          align-items: center;
          gap: 0.5rem;
          margin-top: 1rem;
          transition:
            border-color 0.2s,
            background-color 0.2s;
        }
        .test-button:hover {
          border-color: oklch(0.25 0.008 240.1);
          background: oklch(0.15 0.008 240.1);
        }
        .spinner {
          animation: spin 1s linear infinite;
        }
        @keyframes spin {
          from {
            transform: rotate(0deg);
          }
          to {
            transform: rotate(360deg);
          }
        }
      `}</style>

      <div className="error-page">
        <div className="error-container">
          {/* Main Error Card */}
          <div className="error-card error-card-destructive">
            <div className="error-header">
              <div className="error-icon">
                <svg
                  width="32"
                  height="32"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  style={{ color: 'oklch(0.73 0.127 15.27)' }}
                >
                  <path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z" />
                  <path d="M12 9v4" />
                  <path d="m12 17 .01 0" />
                </svg>
              </div>
              <h1 className="error-title">Backend Unavailable</h1>
              <p className="error-description">
                Unable to connect to the tvarr backend service. The application requires a
                running backend to function properly.
              </p>
            </div>
            <div className="error-content">
              {/* Connection Details */}
              <div className="details-box">
                <div className="details-item">
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <rect width="20" height="8" x="2" y="2" />
                    <rect width="20" height="8" x="2" y="14" />
                    <line x1="6" x2="6.01" y1="6" y2="6" />
                    <line x1="6" x2="6.01" y1="18" y2="18" />
                  </svg>
                  <span>Backend URL:</span>
                </div>
                <div className="url-box">{displayUrl}</div>
                <div className="details-item">
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <path d="M5 12.55a11 11 0 0 1 14.08 0" />
                    <path d="M1.42 9a16 16 0 0 1 21.16 0" />
                    <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
                    <line x1="12" x2="12.01" y1="20" y2="20" />
                  </svg>
                  <span>Status:</span>
                  <span className="status-badge">Connection Failed</span>
                </div>
              </div>

              {/* Retry Section */}
              <div className="retry-section">
                <button onClick={handleRetry} disabled={isRetrying} className="retry-button">
                  {isRetrying ? (
                    <>
                      <svg
                        className="spinner"
                        width="16"
                        height="16"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
                        <path d="M21 3v5h-5" />
                        <path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" />
                        <path d="M3 21v-5h5" />
                      </svg>
                      Retrying...
                    </>
                  ) : (
                    <>
                      <svg
                        width="16"
                        height="16"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                      >
                        <path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
                        <path d="M21 3v5h-5" />
                        <path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" />
                        <path d="M3 21v-5h5" />
                      </svg>
                      Retry Connection
                    </>
                  )}
                </button>

                {retryCount > 0 && (
                  <div className="retry-info">
                    Attempt {retryCount + 1}
                    {retryCount < 5 && ' • Auto-retry in 30s'}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Troubleshooting Card */}
          <div className="error-card">
            <div className="error-content" style={{ paddingTop: '1.5rem' }}>
              <h2 className="troubleshooting-title">
                <svg
                  width="20"
                  height="20"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                  <circle cx="12" cy="12" r="3" />
                </svg>
                Troubleshooting
              </h2>
              <div className="troubleshooting-content">
                <div className="troubleshooting-section">
                  <div className="troubleshooting-heading">Common Solutions:</div>
                  <ul className="troubleshooting-list">
                    <li>Ensure the tvarr backend service is running</li>
                    <li>Check that the backend is accessible at the configured URL</li>
                    <li>Verify firewall settings are not blocking the connection</li>
                    <li>Confirm the backend health endpoint responds to requests</li>
                  </ul>
                </div>

                <div className="troubleshooting-section">
                  <div className="troubleshooting-heading">Configuration:</div>
                  <ul className="troubleshooting-list">
                    <li>
                      Set <span className="code">NEXT_PUBLIC_BACKEND_URL</span> environment variable
                    </li>
                    <li>
                      Default fallback URL is <span className="code">http://localhost:8080</span>
                    </li>
                  </ul>
                </div>

                <a
                  href={`${displayUrl}/health`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="test-button"
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  >
                    <path d="M15 3h6v6" />
                    <path d="M10 14 21 3" />
                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                  </svg>
                  Test Health Endpoint
                </a>
              </div>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
