#!/usr/bin/env python3
import argparse
import json
import os
import sys
from pathlib import Path
from urllib.parse import urlparse, parse_qs

try:
    import msal  # type: ignore
    import requests  # type: ignore
except Exception:
    print("Missing dependencies. Install with: pip3 install msal requests", file=sys.stderr)
    sys.exit(2)

SCOPES = ["User.Read", "Mail.Send"]


def env(name: str) -> str:
    v = os.getenv(name)
    if not v:
        print(f"Missing required env var: {name}", file=sys.stderr)
        sys.exit(2)
    return v


def get_app_and_cache(cache_path: Path):
    tenant_id = env("TENANT_ID")
    client_id = env("CLIENT_ID")
    client_secret = env("CLIENT_SECRET")

    authority = f"https://login.microsoftonline.com/{tenant_id}"

    cache = msal.SerializableTokenCache()
    if cache_path.exists():
        cache.deserialize(cache_path.read_text(encoding="utf-8"))

    app = msal.ConfidentialClientApplication(
        client_id=client_id,
        authority=authority,
        client_credential=client_secret,
        token_cache=cache,
    )
    return app, cache


def persist_cache(cache, path: Path):
    if cache.has_state_changed:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(cache.serialize(), encoding="utf-8")


def acquire_token(app, cache_path: Path, redirected_url: str = "", auth_code: str = ""):
    accounts = app.get_accounts()
    if accounts:
        token = app.acquire_token_silent(SCOPES, account=accounts[0])
        if token and "access_token" in token:
            return token

    redirect_uri = os.getenv("REDIRECT_URI", "http://localhost")

    if auth_code:
        token = app.acquire_token_by_authorization_code(
            code=auth_code,
            scopes=SCOPES,
            redirect_uri=redirect_uri,
        )
        if "access_token" not in token:
            print(json.dumps(token, indent=2), file=sys.stderr)
            raise RuntimeError("Authentication failed")
        return token

    print("No cached token found. Starting authorization-code login flow...", file=sys.stderr)
    flow = app.initiate_auth_code_flow(scopes=SCOPES, redirect_uri=redirect_uri)
    auth_url = flow.get("auth_uri")
    if not auth_url:
        raise RuntimeError("Failed to build auth URL")

    print("Open this URL in your browser and sign in:", file=sys.stderr)
    print(auth_url, file=sys.stderr)
    if redirected_url:
        redirected = redirected_url.strip()
    else:
        print("After sign-in, copy the FULL redirected URL from browser address bar and paste it here.", file=sys.stderr)
        redirected = input("Redirected URL: ").strip()
    parsed = urlparse(redirected)
    auth_response = {k: v[0] for k, v in parse_qs(parsed.query).items()}

    token = app.acquire_token_by_auth_code_flow(flow, auth_response=auth_response)
    if "access_token" not in token:
        print(json.dumps(token, indent=2), file=sys.stderr)
        raise RuntimeError("Authentication failed")

    return token


def parse_recipients(value: str):
    parts = []
    for chunk in (value or "").replace(";", ",").split(","):
        addr = chunk.strip()
        if addr:
            parts.append(addr)
    return parts


def send_mail(access_token: str, to: str, subject: str, body: str, content_type: str = "HTML", cc: str = "", bcc: str = ""):
    url = "https://graph.microsoft.com/v1.0/me/sendMail"

    to_recipients = [
        {"emailAddress": {"address": addr}}
        for addr in parse_recipients(to)
    ]
    cc_recipients = [
        {"emailAddress": {"address": addr}}
        for addr in parse_recipients(cc)
    ]
    bcc_recipients = [
        {"emailAddress": {"address": addr}}
        for addr in parse_recipients(bcc)
    ]

    if not to_recipients:
        raise RuntimeError("At least one recipient is required in --to")

    payload = {
        "message": {
            "subject": subject,
            "body": {
                "contentType": content_type,
                "content": body,
            },
            "toRecipients": to_recipients,
            "ccRecipients": cc_recipients,
            "bccRecipients": bcc_recipients,
        },
        "saveToSentItems": True,
    }

    resp = requests.post(
        url,
        headers={
            "Authorization": f"Bearer {access_token}",
            "Content-Type": "application/json",
        },
        json=payload,
        timeout=30,
    )
    if resp.status_code != 202:
        raise RuntimeError(f"Send failed: {resp.status_code} {resp.text}")


def main():
    parser = argparse.ArgumentParser(description="Send Outlook email through Microsoft Graph")
    parser.add_argument("--to", required=True, help="Recipient email(s), comma/semicolon separated")
    parser.add_argument("--cc", default="", help="CC email(s), comma/semicolon separated")
    parser.add_argument("--bcc", default="", help="BCC email(s), comma/semicolon separated")
    parser.add_argument("--subject", required=True)
    parser.add_argument("--body", required=True, help="Email body text/HTML")
    parser.add_argument("--text", action="store_true", help="Send as plain text instead of HTML")
    parser.add_argument("--cache", default=str(Path.home() / ".openclaw" / "outlook_token_cache.json"))
    parser.add_argument("--redirected-url", default="", help="Optional full redirect URL from browser after auth")
    parser.add_argument("--auth-code", default="", help="Optional OAuth authorization code")
    args = parser.parse_args()

    cache_path = Path(args.cache)
    app, cache = get_app_and_cache(cache_path)
    token = acquire_token(app, cache_path, redirected_url=args.redirected_url, auth_code=args.auth_code)
    persist_cache(cache, cache_path)

    send_mail(
        access_token=token["access_token"],
        to=args.to,
        cc=args.cc,
        bcc=args.bcc,
        subject=args.subject,
        body=args.body,
        content_type="Text" if args.text else "HTML",
    )
    details = []
    if args.cc:
        details.append(f"cc: {args.cc}")
    if args.bcc:
        details.append(f"bcc: {args.bcc}")

    if details:
        print(f"Email queued successfully to {args.to} ({'; '.join(details)})")
    else:
        print(f"Email queued successfully to {args.to}")


if __name__ == "__main__":
    main()
