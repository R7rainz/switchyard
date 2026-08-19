import { betterAuth } from "better-auth";
import { nextCookies } from "better-auth/next-js";
import { jwt } from "better-auth/plugins";
import nodemailer from "nodemailer";
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

function smtpTransport() {
  const host = process.env.SMTP_HOST;
  const port = Number(process.env.SMTP_PORT ?? 587);
  const user = process.env.SMTP_USER;
  const password = process.env.SMTP_PASSWORD;
  const from = process.env.SMTP_FROM;

  if (!host || !Number.isInteger(port) || port <= 0 || !user || !password || !from) {
    throw new Error("SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASSWORD, and SMTP_FROM must be set");
  }

  return {
    from,
    transporter: nodemailer.createTransport({
      host,
      port,
      secure: port === 465,
      auth: { user, pass: password },
    }),
  };
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
  emailAndPassword: {
    enabled: true,
    sendResetPassword: async ({ user, url }) => {
      if (!process.env.SMTP_HOST) {
        if (process.env.NODE_ENV === "production") {
          throw new Error("Password reset email delivery is not configured");
        }
        console.info(`[auth] Password reset link for ${user.email}: ${url}`);
        return;
      }

      const { from, transporter } = smtpTransport();
      await transporter.sendMail({
        from,
        to: user.email,
        subject: "Reset your Switchyard password",
        text: `Reset your Switchyard password using this link:\n\n${url}\n\nThis link expires shortly.`,
      });
    },
  },
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
