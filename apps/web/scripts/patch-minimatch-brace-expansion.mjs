import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";

const minimatchDirectory = join(process.cwd(), "node_modules", "minimatch");
const packagePath = join(minimatchDirectory, "package.json");
const sourcePath = join(minimatchDirectory, "minimatch.js");

let packageJSON;

try {
  packageJSON = JSON.parse(await readFile(packagePath, "utf8"));
} catch (error) {
  if (error?.code === "ENOENT") {
    console.log("minimatch compatibility patch: no hoisted legacy minimatch found");
    process.exit(0);
  }

  throw error;
}

if (!String(packageJSON.version).startsWith("3.")) {
  console.log(
    `minimatch compatibility patch: ${packageJSON.version} does not need the legacy adapter`,
  );
  process.exit(0);
}

const legacyImport = "var expand = require('brace-expansion')";
const compatibleImport = [
  "var braceExpansion = require('brace-expansion')",
  "var expand = typeof braceExpansion === 'function'",
  "  ? braceExpansion : braceExpansion.expand",
].join("\n");
const source = await readFile(sourcePath, "utf8");

if (source.includes(compatibleImport)) {
  console.log(
    `minimatch compatibility patch: ${packageJSON.version} already adapted`,
  );
  process.exit(0);
}

if (!source.includes(legacyImport)) {
  throw new Error(
    `minimatch compatibility patch: unsupported ${packageJSON.version} source; review the adapter before installing`,
  );
}

await writeFile(sourcePath, source.replace(legacyImport, compatibleImport));
console.log(
  `minimatch compatibility patch: adapted ${packageJSON.version} for brace-expansion 5`,
);
