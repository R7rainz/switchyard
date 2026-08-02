// Exercises the auth path the Go backend depends on: sign up, mint a JWT,
// then verify it against the published JWKS exactly as the backend will.
//
// Needs a reachable DATABASE_URL with the migration applied.
//
//   pnpm verify:auth
import assert from "node:assert/strict";

import { createLocalJWKSet, jwtVerify } from "jose";

import { auth } from "../src/lib/auth.ts";

const baseUrl = process.env.BETTER_AUTH_URL ?? "http://localhost:3000";
const call = (path, init) =>
  auth.handler(new Request(`${baseUrl}/api/auth${path}`, init));

const email = `verify-${Date.now()}@switchyard.test`;

const signUp = await call("/sign-up/email", {
  method: "POST",
  headers: { "content-type": "application/json" },
  body: JSON.stringify({ email, password: "correct-horse-battery", name: "Verify" }),
});
// A Response body reads once, and assert evaluates its message eagerly — so
// take the text up front and assert against that.
assert.equal(signUp.status, 200, `sign-up failed: ${await signUp.text()}`);

const cookie = signUp.headers.getSetCookie().join("; ");
assert.ok(cookie, "sign-up set no session cookie");

const tokenRes = await call("/token", { headers: { cookie } });
const tokenBody = await tokenRes.text();
assert.equal(tokenRes.status, 200, `token failed: ${tokenBody}`);
const { token } = JSON.parse(tokenBody);
assert.ok(token, "no token in response");

const jwksRes = await call("/jwks");
assert.equal(jwksRes.status, 200, "jwks failed");
const jwks = await jwksRes.json();
assert.ok(jwks.keys?.length, "jwks published no keys");

// This is the Go backend's job, done here in JS: signature, issuer, audience.
const { payload } = await jwtVerify(token, createLocalJWKSet(jwks), {
  issuer: baseUrl,
  audience: "switchyard-backend",
});
assert.ok(payload.sub, "token carries no subject");

console.log(`ok — verified JWT for ${payload.sub} against ${jwks.keys.length} JWKS key(s)`);
process.exit(0);
