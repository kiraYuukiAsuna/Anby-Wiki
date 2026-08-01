import { readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { setTimeout } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const generatedDir = resolve(scriptDir, "../../../contracts/generated/typescript");

async function writeNormalized(path, content) {
  for (let attempt = 0; ; attempt += 1) {
    try {
      await writeFile(path, content);
      return;
    } catch (error) {
      if (
        attempt >= 5 ||
        !["EBUSY", "EPERM", "UNKNOWN"].includes(error?.code)
      ) {
        throw error;
      }
      await setTimeout(25 * 2 ** attempt);
    }
  }
}

async function normalizeDirectory(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      await normalizeDirectory(path);
      continue;
    }
    if (
      !entry.isFile() ||
      (!entry.name.endsWith(".md") && !entry.name.endsWith(".ts"))
    ) {
      continue;
    }

    const original = await readFile(path, "utf8");
    const normalized = original.replace(/[ \t]+$/gm, "");
    if (normalized !== original) {
      await writeNormalized(path, normalized);
    }
  }
}

await normalizeDirectory(generatedDir);
