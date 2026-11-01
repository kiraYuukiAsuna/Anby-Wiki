#!/usr/bin/env python3
"""Evaluate a fixed relevance manifest in a fresh, disposable Meilisearch index."""
import argparse
import json
import math
import os
from pathlib import Path
import random
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--index", required=True)
    parser.add_argument("--manifest", type=Path, default=Path(__file__).with_name("search-zh.json"))
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    if not args.index.startswith("wiki_perf_"):
        parser.error("index must start with wiki_perf_")
    manifest = json.loads(args.manifest.read_text())
    documents = manifest["documents"]
    queries = manifest["queries"]
    ids = {doc["id"] for doc in documents}
    if manifest["version"] != 1 or len(ids) != len(documents) or not queries:
        parser.error("invalid manifest")
    if any(not query["expected"] or not set(query["expected"]) <= ids for query in queries):
        parser.error("query labels must refer to documents")

    def request(method, path, value=None):
        payload = json.dumps(value, ensure_ascii=False).encode() if value is not None else None
        headers = {"Content-Type": "application/json"}
        if key := os.environ.get("MEILI_API_KEY"):
            headers["Authorization"] = "Bearer " + key
        req = urllib.request.Request(args.base_url.rstrip("/") + path, payload,
                                     headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=15) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            # Never include provider keys or response messages in reports.
            try:
                code = json.load(error).get("code", "unknown")
            except (ValueError, AttributeError):
                code = "unknown"
            raise RuntimeError(f"Meilisearch HTTP {error.code}: {code}") from None

    def wait(task):
        deadline = time.monotonic() + 600
        while time.monotonic() < deadline:
            row = request("GET", "/tasks/" + str(task["taskUid"]))
            if row["status"] == "succeeded":
                return
            if row["status"] in ("failed", "canceled"):
                raise RuntimeError("Meilisearch task failed: " + row["error"]["code"])
            time.sleep(0.25)
        raise TimeoutError("Meilisearch task deadline")

    existing = request("GET", "/indexes?limit=1000")["results"]
    if any(index["uid"] == args.index for index in existing):
        parser.error("index already exists; refusing to overwrite")
    path = "/indexes/" + args.index
    owned = False
    rows = []
    try:
        created = request("POST", "/indexes", {"uid": args.index, "primaryKey": "page_id"})
        wait(created)
        owned = True
        attributes = ["display_title", "normalized_title", "aliases", "body", "entity_terms"]
        wait(request("PATCH", path + "/settings", {
            "searchableAttributes": attributes,
            "filterableAttributes": ["wiki_id", "namespace", "language", "entity_type"],
            "pagination": {"maxTotalHits": 100000},
            "faceting": {"maxValuesPerFacet": 100},
            "embedders": {"page-content": {
                "source": "huggingFace",
                "model": "sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2",
                "documentTemplate": "{{doc.display_title}}\n{{doc.body}}",
            }},
        }))
        wiki_id = str(uuid.uuid5(uuid.NAMESPACE_URL, args.index))
        page_ids = {doc["id"]: str(uuid.uuid5(uuid.NAMESPACE_URL, args.index + "/" + doc["id"]))
                    for doc in documents}
        labels = {value: key for key, value in page_ids.items()}
        wait(request("PUT", path + "/documents?primaryKey=page_id", [{
            "page_id": page_ids[doc["id"]], "wiki_id": wiki_id,
            "namespace": "main", "language": "zh-Hans",
            "source_revision_id": str(uuid.uuid4()),
            "display_title": doc["title"], "normalized_title": doc["title"].lower(),
            "body": doc["body"], "aliases": [], "entity_id": None,
            "entity_type": "technical-topic", "entity_terms": [],
        } for doc in documents]))
        for repeat in range(3):
            order = list(queries)
            random.Random(repeat).shuffle(order)
            for query in order:
                for mode, ratio in (("keyword", 0), ("hybrid", 0.5), ("semantic", 1)):
                    body = {
                        "q": query["text"], "limit": 5, "offset": 0,
                        "filter": f'wiki_id = "{wiki_id}"',
                        "attributesToSearchOn": attributes,
                        "attributesToHighlight": attributes,
                        "highlightPreTag": "[[", "highlightPostTag": "]]",
                        "showRankingScore": True,
                        "facets": ["namespace", "language", "entity_type"],
                        "attributesToCrop": ["body"], "cropLength": 24, "cropMarker": "...",
                    }
                    if ratio:
                        body["hybrid"] = {"embedder": "page-content", "semanticRatio": ratio}
                    started = time.monotonic()
                    result = request("POST", path + "/search", body)
                    hits = [labels[hit["page_id"]] for hit in result["hits"]]
                    expected = set(query["expected"])
                    ranks = [i + 1 for i, hit in enumerate(hits) if hit in expected]
                    ideal = sum(1 / math.log2(i + 2) for i in range(min(5, len(expected))))
                    rows.append({
                        "repeat": repeat, "query": query["id"], "kind": query["kind"],
                        "mode": mode, "hits": hits,
                        "recall_at_5": len(ranks) / len(expected),
                        "mrr": 1 / min(ranks) if ranks else 0,
                        "ndcg_at_5": sum(1 / math.log2(rank + 1) for rank in ranks) / ideal,
                        "latency_ms": (time.monotonic() - started) * 1000,
                    })
        summary = []
        for mode in ("keyword", "hybrid", "semantic"):
            for kind in ("all", *sorted({query["kind"] for query in queries})):
                group = [row for row in rows if row["mode"] == mode
                         and (kind == "all" or row["kind"] == kind)]
                summary.append({"mode": mode, "kind": kind, "requests": len(group), **{
                    metric: sum(row[metric] for row in group) / len(group)
                    for metric in ("recall_at_5", "mrr", "ndcg_at_5")
                }})
        passed = True
        for repeat in range(3):
            for kind in {query["kind"] for query in queries}:
                group = [row for row in rows if row["repeat"] == repeat
                         and row["mode"] == "semantic" and row["kind"] == kind]
                passed = passed and (
                    sum(row["recall_at_5"] for row in group) / len(group)
                    >= manifest["minimum_semantic_recall_at_5"]
                    and sum(row["mrr"] for row in group) / len(group)
                    >= manifest["minimum_semantic_mrr"]
                )
        args.report.write_text(json.dumps({
            "version": 1, "manifest": str(args.manifest), "description": manifest["description"],
            "documents": len(documents), "queries": len(queries), "repeats": 3,
            "minimum_semantic_recall_at_5": manifest["minimum_semantic_recall_at_5"],
            "minimum_semantic_mrr": manifest["minimum_semantic_mrr"],
            "gate_scope": "every query kind in every repeat",
            "passed": passed, "summary": summary, "rows": rows,
        }, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"passed": passed, "summary": summary}, ensure_ascii=False))
        return 0 if passed else 1
    finally:
        if owned:
            wait(request("DELETE", path))


if __name__ == "__main__":
    raise SystemExit(main())
