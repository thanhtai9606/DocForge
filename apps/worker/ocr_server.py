#!/usr/bin/env python3
"""DocForge OCR sidecar (CPU). Rasterize one PDF page and run Tesseract.

Does not read or write job/DB/orchestrator state. Go worker calls POST /v1/ocr.
"""
from __future__ import annotations

import base64
import json
import os
import subprocess
import tempfile
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ADDR = os.environ.get("OCR_ADDR", "0.0.0.0:8090")
DPI = int(os.environ.get("OCR_DPI", "200"))
TIMEOUT = int(os.environ.get("OCR_TIMEOUT_SEC", "60"))
MAX_BODY = int(os.environ.get("OCR_MAX_BODY", str(80 * 1024 * 1024)))

LANG_MAP = {"vi": "vie", "en": "eng", "vie": "vie", "eng": "eng", "vn": "vie"}


def tess_langs(langs) -> str:
    out: list[str] = []
    seen: set[str] = set()
    for item in langs or ["eng"]:
        code = LANG_MAP.get(str(item).lower(), "eng")
        if code not in seen:
            seen.add(code)
            out.append(code)
    return "+".join(out) if out else "eng"


def tess_version() -> str:
    try:
        proc = subprocess.run(
            ["tesseract", "--version"], capture_output=True, text=True, timeout=5, check=False
        )
        text = proc.stdout or proc.stderr
        first = text.splitlines()[0] if text else "unknown"
        return first.replace("tesseract ", "").strip()
    except Exception:
        return "unknown"


def render_page(pdf_bytes: bytes, page: int) -> bytes:
    with tempfile.TemporaryDirectory(prefix="docforge-ocr-") as td:
        pdf_path = Path(td) / "in.pdf"
        pdf_path.write_bytes(pdf_bytes)
        prefix = Path(td) / "page"
        subprocess.run(
            [
                "pdftoppm",
                "-f",
                str(page),
                "-l",
                str(page),
                "-png",
                "-r",
                str(DPI),
                str(pdf_path),
                str(prefix),
            ],
            check=True,
            capture_output=True,
            timeout=TIMEOUT,
        )
        pngs = sorted(Path(td).glob("page*.png"))
        if not pngs:
            raise RuntimeError("pdftoppm produced no image")
        return pngs[0].read_bytes()


def parse_tsv(tsv: str) -> tuple[str, float]:
    lines = tsv.splitlines()
    if len(lines) < 2:
        return tsv.strip(), 0.0
    header = lines[0].split("\t")
    try:
        conf_i = header.index("conf")
        text_i = header.index("text")
    except ValueError:
        return tsv.strip(), 0.0
    words: list[str] = []
    confs: list[float] = []
    for line in lines[1:]:
        cols = line.split("\t")
        if len(cols) <= max(conf_i, text_i):
            continue
        text = cols[text_i].strip()
        try:
            conf = float(cols[conf_i])
        except ValueError:
            continue
        if conf < 0 or not text:
            continue
        words.append(text)
        confs.append(conf / 100.0)
    avg = sum(confs) / len(confs) if confs else 0.0
    return " ".join(words), avg


def ocr_image(png: bytes, langs) -> tuple[str, float]:
    with tempfile.TemporaryDirectory(prefix="docforge-tess-") as td:
        img = Path(td) / "page.png"
        img.write_bytes(png)
        out_base = Path(td) / "out"
        subprocess.run(
            ["tesseract", str(img), str(out_base), "-l", tess_langs(langs), "--psm", "3", "tsv"],
            check=True,
            capture_output=True,
            timeout=TIMEOUT,
        )
        tsv = Path(str(out_base) + ".tsv").read_text(encoding="utf-8", errors="replace")
        return parse_tsv(tsv)


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt: str, *args) -> None:
        print(f"[ocr] {self.address_string()} {fmt % args}", flush=True)

    def _json(self, code: int, payload: dict) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802
        if self.path.split("?", 1)[0] in ("/healthz", "/health", "/"):
            self._json(200, {"status": "ok", "provider": "tesseract", "version": tess_version()})
            return
        self._json(404, {"error": "not found"})

    def do_POST(self) -> None:  # noqa: N802
        path = self.path.split("?", 1)[0]
        if path not in ("/v1/ocr", "/ocr"):
            self._json(404, {"error": "not found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = 0
        if length <= 0 or length > MAX_BODY:
            self._json(400, {"error": "invalid body"})
            return
        try:
            req = json.loads(self.rfile.read(length))
        except json.JSONDecodeError:
            self._json(400, {"error": "invalid json"})
            return
        try:
            page = int(req.get("page_number") or 0)
        except (TypeError, ValueError):
            page = 0
        if page < 1:
            self._json(400, {"error": "page_number must be >= 1"})
            return
        langs = req.get("language") or ["eng"]
        if isinstance(langs, str):
            langs = [langs]
        pdf_b64 = req.get("pdf_base64") or req.get("payload_base64") or ""
        if not pdf_b64:
            self._json(400, {"error": "pdf_base64 required"})
            return
        try:
            pdf = base64.b64decode(pdf_b64)
        except Exception:
            self._json(400, {"error": "invalid pdf_base64"})
            return
        try:
            png = render_page(pdf, page)
            text, conf = ocr_image(png, langs)
        except subprocess.CalledProcessError as exc:
            detail = (exc.stderr or exc.stdout or b"").decode("utf-8", errors="replace")[:500]
            self._json(502, {"error": "ocr failed", "detail": detail})
            return
        except Exception as exc:
            self._json(502, {"error": str(exc)})
            return
        detected = langs[0] if langs else "und"
        self._json(
            200,
            {
                "text": text,
                "confidence": round(conf, 4),
                "language": LANG_MAP.get(str(detected).lower(), str(detected)),
                "provider": "tesseract",
                "version": tess_version(),
                "page_number": page,
            },
        )


def main() -> None:
    host, _, port_s = ADDR.rpartition(":")
    if not host:
        host = "0.0.0.0"
    httpd = ThreadingHTTPServer((host, int(port_s)), Handler)
    print(f"[ocr] listening on {host}:{port_s}", flush=True)
    httpd.serve_forever()


if __name__ == "__main__":
    main()
