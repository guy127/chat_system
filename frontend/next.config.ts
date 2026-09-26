import type { NextConfig } from "next";

// In Docker, Nginx routes /api and /ws to the Go server. For `npm run dev`
// without Nginx, /api is proxied here and the WebSocket goes straight to
// NEXT_PUBLIC_WS_URL (see .env.development).
const apiURL = process.env.API_INTERNAL_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  poweredByHeader: false,
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiURL}/:path*` }];
  },
};

export default nextConfig;
