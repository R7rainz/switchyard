/**
 * Base URL of the Go API server.
 *
 * NEXT_PUBLIC_ because the browser calls the backend directly with a minted
 * JWT — nothing about this request goes through Next.
 */
export const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8090";
