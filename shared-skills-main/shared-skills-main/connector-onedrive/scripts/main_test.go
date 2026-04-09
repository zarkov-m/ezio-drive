package main

import "testing"

func TestEncodePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/", "/"},
		{"", "/"},
		{"Documents/file name.txt", "/Documents/file%20name.txt"},
		{"/A/B+C", "/A/B+C"},
	}

	for _, tc := range cases {
		got := encodePath(tc.in)
		if got != tc.want {
			t.Fatalf("encodePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestClamp(t *testing.T) {
	if got := clamp(0, 1, 10); got != 1 {
		t.Fatalf("clamp low = %d", got)
	}
	if got := clamp(50, 1, 10); got != 10 {
		t.Fatalf("clamp high = %d", got)
	}
	if got := clamp(5, 1, 10); got != 5 {
		t.Fatalf("clamp mid = %d", got)
	}
}

func TestNormalizeFolderPath(t *testing.T) {
	it := driveItem{Name: "Downloads"}
	it.ParentReference.Path = "/drive/root:/Users/henry"

	if got := normalizeFolderPath("/Inbox", it); got != "/Inbox" {
		t.Fatalf("normalize explicit path = %q", got)
	}

	if got := normalizeFolderPath("id:abc", it); got != "/Users/henry/Downloads" {
		t.Fatalf("normalize id path = %q", got)
	}
}


func TestEncodeSharingURL(t *testing.T) {
	got := encodeSharingURL("https://contoso.sharepoint.com/:f:/s/demo/abc?e=123")
	want := "u!aHR0cHM6Ly9jb250b3NvLnNoYXJlcG9pbnQuY29tLzpmOi9zL2RlbW8vYWJjP2U9MTIz"
	if got != want {
		t.Fatalf("encodeSharingURL() = %q, want %q", got, want)
	}
}

func TestSplitSharePointServerRelativePath(t *testing.T) {
	sitePath, itemPath, ok := splitSharePointServerRelativePath("/sites/DMC/Shared Documents/PayPal Co-Marketing Articles")
	if !ok {
		t.Fatal("expected ok")
	}
	if sitePath != "/sites/DMC" {
		t.Fatalf("sitePath = %q", sitePath)
	}
	if itemPath != "/PayPal Co-Marketing Articles" {
		t.Fatalf("itemPath = %q", itemPath)
	}
}

func TestSplitSharePointPersonalServerRelativePath(t *testing.T) {
	sitePath, itemPath, ok := splitSharePointServerRelativePath("/personal/angel_example_com/Documents/Shared Folder")
	if !ok {
		t.Fatal("expected ok")
	}
	if sitePath != "/personal/angel_example_com" {
		t.Fatalf("sitePath = %q", sitePath)
	}
	if itemPath != "/Shared Folder" {
		t.Fatalf("itemPath = %q", itemPath)
	}
}
