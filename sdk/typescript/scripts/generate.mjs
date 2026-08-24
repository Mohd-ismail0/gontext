#!/usr/bin/env node
/**
 * Regenerates OpenAPI types into src/generated/openapi.ts when openapi-typescript is installed.
 * The hand-written client in src/index.ts remains the supported surface for v1.
 */
import { spawnSync } from "node:child_process";
import { mkdirSync, existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "../..", "..");
const openapi = join(root, "contracts", "openapi", "openapi.yaml");
const outDir = join(here, "..", "src", "generated");
const outFile = join(outDir, "openapi.ts");

if (!existsSync(openapi)) {
  console.error("openapi.yaml not found at", openapi);
  process.exit(1);
}

mkdirSync(outDir, { recursive: true });

const bin = join(here, "..", "node_modules", ".bin", "openapi-typescript");
const cmd = existsSync(bin) ? bin : "npx";
const args = existsSync(bin)
  ? [openapi, "-o", outFile]
  : ["--yes", "openapi-typescript", openapi, "-o", outFile];

const r = spawnSync(cmd, args, { stdio: "inherit", shell: true });
if (r.status !== 0) {
  console.error(
    "generate failed — install devDeps (npm i) or keep using the hand-written client in src/index.ts",
  );
  process.exit(r.status ?? 1);
}
console.log("wrote", outFile);
