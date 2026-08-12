package importer

import "testing"

func TestInferSourceMetadataFromHTML(t *testing.T) {
	source := &AcquiredSource{
		URL:      "https://www.rfc-editor.org/rfc/rfc7523.html",
		Filename: "rfc7523.html",
		MIMEType: "text/html",
		Content: []byte(`<!doctype html><html><head>
			<title>fallback title</title>
			<meta name="citation_title" content="JSON Web Token (JWT) Profile for OAuth 2.0">
			<meta name="citation_author" content="Michael B. Jones">
			<meta property="og:site_name" content="RFC Editor">
			<meta name="citation_publication_date" content="2015-05-01">
			<meta name="citation_doi" content="10.17487/RFC7523">
		</head></html>`),
	}

	metadata := inferSourceMetadata(source)
	if metadata.Title != "JSON Web Token (JWT) Profile for OAuth 2.0" ||
		metadata.Author != "Michael B. Jones" ||
		metadata.Publisher != "RFC Editor" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
	if metadata.PublishedAt == nil || metadata.PublishedAt.Format("2006-01-02") != "2015-05-01" {
		t.Fatalf("unexpected publication date: %#v", metadata.PublishedAt)
	}
	if string(metadata.Metadata) == "{}" {
		t.Fatalf("expected bounded provenance metadata: %s", metadata.Metadata)
	}
}

func TestInferSourceMetadataFromPlainTextFrontMatter(t *testing.T) {
	metadata := inferSourceMetadata(&AcquiredSource{
		Filename: "document.md",
		MIMEType: "text/plain",
		Content:  []byte("---\ntitle: A grounded article\nauthor: Ada Example\ndate: 2026-08-12\n---\n# Ignored fallback\n"),
	})
	if metadata.Title != "A grounded article" || metadata.Author != "Ada Example" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
}

func TestInferSourceMetadataFromRFCText(t *testing.T) {
	metadata := inferSourceMetadata(&AcquiredSource{
		Filename: "rfc7523.txt",
		MIMEType: "text/plain",
		Content: []byte(`Internet Engineering Task Force (IETF)                 M. Jones
Request for Comments: 7523                                  Microsoft
Category: Standards Track                                      May 2015
ISSN: 2070-1721

     JSON Web Token (JWT) Profile for OAuth 2.0 Client
                  Authentication and Authorization Grants

Abstract

This document defines a profile.`),
	})
	if metadata.Title != "JSON Web Token (JWT) Profile for OAuth 2.0 Client Authentication and Authorization Grants" ||
		metadata.Publisher != "RFC Editor" {
		t.Fatalf("unexpected RFC metadata: %#v", metadata)
	}
	if metadata.PublishedAt == nil || metadata.PublishedAt.Format("2006-01") != "2015-05" {
		t.Fatalf("unexpected RFC date: %#v", metadata.PublishedAt)
	}
}
