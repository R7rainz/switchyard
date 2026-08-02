// Prints the Better Auth schema as SQL. Needs a reachable DATABASE_URL: the
// compiler diffs against the live database and emits only what is missing.
//
// The standalone @better-auth/cli lags the library by a minor version, so this
// calls the library's own migration compiler instead.
//
//   pnpm gen:schema > ../backend/migrations/0001_better_auth.sql
import { getMigrations } from "better-auth/db/migration";

import { auth } from "../src/lib/auth.ts";

const { compileMigrations } = await getMigrations(auth.options);
process.stdout.write(await compileMigrations());
process.exit(0);
