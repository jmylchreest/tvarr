/** @type {import('next').NextConfig} */
const nextConfig = {
  output: process.env.NODE_ENV === 'production' ? 'export' : undefined,
  trailingSlash: true,
  images: {
    unoptimized: true,
  },
  // Only add rewrites in development mode (not needed for static export)
  ...(process.env.NODE_ENV !== 'production' && {
    async rewrites() {
      return [
        {
          source: '/api/v1/:path*',
          destination: 'http://localhost:8080/api/v1/:path*',
        },
        {
          source: '/health',
          destination: 'http://localhost:8080/health',
        },
        {
          source: '/live',
          destination: 'http://localhost:8080/live',
        },
      ];
    },
  }),
};

module.exports = nextConfig;
