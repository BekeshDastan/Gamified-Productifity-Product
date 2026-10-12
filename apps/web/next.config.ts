import type { NextConfig } from "next";
import path from "node:path";

const nextConfig: NextConfig = {
  poweredByHeader: false,
  output: "standalone",
  outputFileTracingRoot: path.resolve(process.cwd(), "../.."),
  // Same-origin browser API; production reverse proxy supplies these routes itself.
  async rewrites() {
    const apiOrigin = process.env.INTERNAL_API_ORIGIN ?? "http://127.0.0.1:8080";
    const parsed = new URL(apiOrigin);
    if (
      !["http:", "https:"].includes(parsed.protocol) ||
      parsed.username ||
      parsed.password ||
      parsed.pathname !== "/" ||
      parsed.search ||
      parsed.hash
    )
      throw new Error("INTERNAL_API_ORIGIN must be an origin without credentials");
    return [
      { source: "/health/:path*", destination: `${parsed.origin}/health/:path*` },
      { source: "/api/v1/:path*", destination: `${parsed.origin}/api/v1/:path*` },
    ];
  },
};

export default nextConfig;
