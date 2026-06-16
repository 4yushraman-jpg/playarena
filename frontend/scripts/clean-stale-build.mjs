// Runs as a `predev` hook. Running `next dev` on top of a previous production
// `next build` output causes phantom 404s on nested routes (the dev router and
// the prebuilt manifest disagree). A production build leaves markers a dev build
// never creates (BUILD_ID, and — with output:"standalone" — a standalone/ dir);
// a normal dev `.next` has neither, so we preserve the dev cache in that case.
import { existsSync, rmSync } from "node:fs"

const isProdBuild = existsSync(".next/BUILD_ID") || existsSync(".next/standalone")

if (isProdBuild) {
  rmSync(".next", { recursive: true, force: true })
  console.log("[predev] removed a stale production .next build (would 404 nested routes in dev)")
}
