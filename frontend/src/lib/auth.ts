import { betterAuth } from "better-auth";
import { nextCookies } from "better-auth/next-js";
import { jwt } from "better-auth/plugins";
import { Pool } from "pg";

const databaseUrl = process.env.DATABASE_URL;
if (!databaseUrl) {
  throw new Error("DATABASE_URL is not set");
}

// Must match the --port in package.json: this becomes the JWT issuer, and the
// Go verifier compares it exactly. URL.origin also removes a trailing slash,
// which is not part of the browser's Origin header.
function originOf(value: string): string {
  try {
    return new URL(value).origin;
  } catch {
    throw new Error("BETTER_AUTH_URL must be an absolute URL");
  }
}

function vercelOrigin(value?: string): string | undefined {
  if (!value) return undefined;
  try {
    return new URL(value.includes("://") ? value : `https://${value}`).origin;
  } catch {
    return undefined;
  }
}

const baseUrl = originOf(process.env.BETTER_AUTH_URL ?? "http://localhost:3007");
const trustedOrigins = [
  baseUrl,
  vercelOrigin(process.env.VERCEL_URL),
  vercelOrigin(process.env.VERCEL_BRANCH_URL),
  vercelOrigin(process.env.VERCEL_PROJECT_PRODUCTION_URL),
].filter((origin): origin is string => Boolean(origin));

/**
 * Better Auth owns the credential flow: signup, login, password hashing, and
 * session lifecycle all live here, in the frontend.
 *
 * The Go backend never sees a password. It verifies the JWT minted by the jwt
 * plugin against the JWKS this app publishes at /api/auth/jwks.
 */
export const auth = betterAuth({
  baseURL: baseUrl,
  trustedOrigins,
  database: new Pool({ connectionString: databaseUrl }),
  emailAndPassword: { enabled: true },
  plugins: [
    jwt({
      jwt: {
        issuer: baseUrl,
        audience: "switchyard-backend",
        expirationTime: "15m",
      },
    }),
    // nextCookies stays last: it wraps the handlers that come before it.
    nextCookies(),
  ],
});
