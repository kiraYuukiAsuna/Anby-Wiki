import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const webRoot = path.join(root, "apps", "web");
const openAPIPath = path.join(root, "contracts", "openapi", "openapi.yaml");

function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules" || entry.name === ".next") return [];
      return walk(absolute);
    }
    return /\.(?:ts|tsx)$/.test(entry.name) ? [absolute] : [];
  });
}

function relative(absolute) {
  return path.relative(root, absolute).split(path.sep).join("/");
}

function parseOperations(source) {
  let currentPath = "";
  let currentMethod = "";
  let currentOperation = null;
  const operations = [];
  for (const line of source.split("\n")) {
    const pathMatch = line.match(/^  (\/[^:]+):\s*$/);
    if (pathMatch) {
      currentPath = pathMatch[1];
      continue;
    }
    const methodMatch = line.match(/^    (get|post|put|patch|delete):\s*$/);
    if (methodMatch) {
      currentMethod = methodMatch[1].toUpperCase();
      continue;
    }
    const operationMatch = line.match(/^      operationId:\s*(\S+)\s*$/);
    if (operationMatch) {
      currentOperation = {
        id: operationMatch[1],
        method: currentMethod,
        path: currentPath,
        cliOnly: false,
      };
      operations.push(currentOperation);
      continue;
    }
    if (
      currentOperation &&
      /^      x-anby-cli-only:\s*true\s*$/.test(line)
    ) {
      currentOperation.cliOnly = true;
    }
  }
  return operations;
}

function resolveImport(from, specifier, fileSet) {
  let base;
  if (specifier.startsWith("@/")) {
    base = path.join(webRoot, specifier.slice(2));
  } else if (specifier.startsWith(".")) {
    base = path.resolve(path.dirname(from), specifier);
  } else {
    return null;
  }
  for (const candidate of [
    base,
    `${base}.ts`,
    `${base}.tsx`,
    path.join(base, "index.ts"),
    path.join(base, "index.tsx"),
  ]) {
    if (fileSet.has(candidate)) return candidate;
  }
  return null;
}

function routeName(file) {
  const relativeFile = relative(file);
  if (relativeFile === "apps/web/app/layout.tsx") return "(global layout)";
  if (!/^apps\/web\/app\/(?:.*\/)?page\.tsx$/.test(relativeFile)) return null;
  const route = relativeFile
    .replace(/^apps\/web\/app/, "")
    .replace(/\/page\.tsx$/, "");
  return route || "/";
}

const operations = parseOperations(fs.readFileSync(openAPIPath, "utf8"));
const operationIDs = operations.map((operation) => operation.id);
const duplicateIDs = operationIDs.filter(
  (id, index) => operationIDs.indexOf(id) !== index,
);
if (duplicateIDs.length > 0) {
  throw new Error(
    `duplicate OpenAPI operationId values: ${[...new Set(duplicateIDs)].join(", ")}`,
  );
}

const files = walk(webRoot);
const fileSet = new Set(files);
const sources = new Map(
  files.map((file) => [file, fs.readFileSync(file, "utf8")]),
);
const reverseImports = new Map();

for (const [file, source] of sources) {
  for (const match of source.matchAll(
    /(?:import|export)\s+(?:[\s\S]*?\s+from\s+)?["']([^"']+)["']/g,
  )) {
    const dependency = resolveImport(file, match[1], fileSet);
    if (!dependency) continue;
    const importers = reverseImports.get(dependency) ?? new Set();
    importers.add(file);
    reverseImports.set(dependency, importers);
  }
}

function owningRoutes(start) {
  const queue = [start];
  const visited = new Set(queue);
  const routes = new Set();
  while (queue.length > 0) {
    const current = queue.shift();
    const route = routeName(current);
    if (route) routes.add(route);
    for (const importer of reverseImports.get(current) ?? []) {
      if (visited.has(importer)) continue;
      visited.add(importer);
      queue.push(importer);
    }
  }
  return [...routes].sort();
}

const failures = [];
const routeOwners = new Set();
let cliOwned = 0;

for (const operation of operations) {
  if (operation.cliOnly) {
    cliOwned += 1;
    continue;
  }
  const callPattern = new RegExp(`\\.\\s*${operation.id}\\s*\\(`);
  const callsites = files.filter((file) => callPattern.test(sources.get(file)));
  const routes = [...new Set(callsites.flatMap(owningRoutes))].sort();
  routes.forEach((route) => routeOwners.add(route));

  if (callsites.length === 0) {
    failures.push(
      `${operation.method} ${operation.path} (${operation.id}): no Web callsite`,
    );
    continue;
  }
  if (routes.length === 0) {
    failures.push(
      `${operation.method} ${operation.path} (${operation.id}): callsite is not imported by a page or global layout`,
    );
  }
}

if (failures.length > 0) {
  console.error("web api coverage: failed");
  failures.forEach((failure) => console.error(`- ${failure}`));
  process.exit(1);
}

console.log(
  `web api coverage: ${operations.length - cliOwned}/${operations.length} Web-owned operations have callsites and routable UI owners (${routeOwners.size} owners); ${cliOwned} operations are Agent CLI transport`,
);
