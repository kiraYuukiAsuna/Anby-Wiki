import { readdir, readFile, writeFile } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const generatedDir = resolve(scriptDir, "../../../contracts/generated/typescript");

async function normalizeDirectory(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  for (const entry of entries) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      await normalizeDirectory(path);
      continue;
    }
    if (!entry.isFile() || !entry.name.endsWith(".md")) continue;

    const original = await readFile(path, "utf8");
    const normalized = original.replace(/[ \t]+$/gm, "");
    if (normalized !== original) {
      await writeFile(path, normalized);
    }
  }
}

await normalizeDirectory(generatedDir);
