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

function escapeHtml(value: string): string {
  const entities: Record<string, string> = {
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  };
  return value.replace(/[&<>"']/g, (character) => entities[character]);
}

function resetPasswordEmail(url: string): string {
  const safeUrl = escapeHtml(url);

  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width">
    <title>Reset your Switchyard password</title>
  </head>
  <body style="margin:0;background:#f6f5f3;color:#111111;font-family:Arial,sans-serif;">
    <div style="padding:32px 16px;">
      <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;margin:0 auto;">
        <tr>
          <td style="padding:0 0 16px;font-size:13px;font-weight:700;letter-spacing:3px;">
            <span style="display:inline-block;width:10px;height:10px;margin-right:8px;background:#e8400d;border-radius:3px;"></span>SWITCHYARD
          </td>
        </tr>
        <tr>
          <td style="background:#ffffff;border:1px solid #ecebea;border-radius:12px;overflow:hidden;">
            <div style="height:4px;background:#e8400d;"></div>
            <div style="padding:36px;">
              <div style="display:inline-block;padding:8px 10px;background:#ffef99;border-radius:8px;font-size:11px;font-weight:700;letter-spacing:1px;text-transform:uppercase;">Account access</div>
              <h1 style="margin:24px 0 12px;font-size:30px;line-height:1.1;letter-spacing:-1px;">Reset your password</h1>
              <p style="margin:0;color:#6d6c6b;font-size:16px;line-height:1.6;">We received a request to set a new password for your Switchyard account.</p>
              <table role="presentation" cellspacing="0" cellpadding="0" style="margin:28px 0 24px;">
                <tr>
                  <td style="border-radius:8px;background:#111111;">
                    <a href="${safeUrl}" style="display:inline-block;padding:14px 20px;color:#ffffff;font-size:14px;font-weight:700;text-decoration:none;">Set a new password&nbsp; &rarr;</a>
                  </td>
                </tr>
              </table>
              <p style="margin:0;color:#6d6c6b;font-size:13px;line-height:1.6;">This link expires shortly and can only be used once.</p>
              <div style="height:1px;margin:28px 0;background:#ecebea;"></div>
              <p style="margin:0;color:#6d6c6b;font-size:12px;line-height:1.6;">If you didn’t request this, you can safely ignore this email.</p>
            </div>
          </td>
        </tr>
        <tr>
          <td style="padding:18px 4px 0;color:#6d6c6b;font-size:12px;line-height:1.6;">
            If the button doesn’t work, copy and paste this link:<br>
            <a href="${safeUrl}" style="color:#111111;word-break:break-all;">${safeUrl}</a>
          </td>
        </tr>
      </table>
    </div>
  </body>
</html>`;
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
        html: resetPasswordEmail(url),
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
