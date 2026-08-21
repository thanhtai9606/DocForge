#!/usr/bin/env python3
"""DocForge OCR sidecar (CPU). Batch rasterize + Tesseract/RapidOCR.

Does not read or write job/DB/orchestrator state. Go worker calls POST /v1/ocr or /v1/ocr/batch.
"""
from __future__ import annotations

import base64
import hashlib
import io
import json
import os
import shutil
import subprocess
import tempfile
import threading
from collections import OrderedDict
from concurrent.futures import ThreadPoolExecutor, as_completed
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

from PIL import Image

ADDR = os.environ.get("OCR_ADDR", "0.0.0.0:8090")
DPI = int(os.environ.get("OCR_DPI", "150"))
TIMEOUT = int(os.environ.get("OCR_TIMEOUT_SEC", "120"))
MAX_BODY = int(os.environ.get("OCR_MAX_BODY", str(80 * 1024 * 1024)))
MAX_CONCURRENT = max(1, int(os.environ.get("OCR_MAX_CONCURRENT", "4")))
PDF_CACHE_SIZE = max(4, int(os.environ.get("OCR_PDF_CACHE_SIZE", "16")))
MAX_EDGE = int(os.environ.get("OCR_MAX_EDGE", "2200"))
OCR_ENGINE = os.environ.get("OCR_ENGINE", "auto").strip().lower()
TESS_OEM = os.environ.get("OCR_TESS_OEM", "1")
TESS_PSM = os.environ.get("OCR_TESS_PSM", "6")

LANG_MAP = {"vi": "vie", "en": "eng", "vie": "vie", "eng": "eng", "vn": "vie"}

_ocr_sem = threading.Semaphore(MAX_CONCURRENT)
_rapid_lock = threading.Lock()
_rapid_engine: Any = None


class PDFCache:
    """Reuse decoded PDF files across concurrent page OCR for one document."""

    def __init__(self, max_entries: int) -> None:
        self._max = max_entries
        self._lock = threading.Lock()
        self._entries: OrderedDict[str, Path] = {}

    def materialize(self, document_id: str, pdf: bytes) -> Path:
        key = document_id.strip() if document_id.strip() else hashlib.sha256(pdf).hexdigest()
        with self._lock:
            cached = self._entries.get(key)
            if cached is not None and cached.exists():
                self._entries.move_to_end(key)
                return cached
            if cached is not None:
                del self._entries[key]

            td = Path(tempfile.mkdtemp(prefix="docforge-pdf-"))
            path = td / "in.pdf"
            path.write_bytes(pdf)
            self._entries[key] = path
            while len(self._entries) > self._max:
                _, old_path = self._entries.popitem(last=False)
                shutil.rmtree(old_path.parent, ignore_errors=True)
            return path


_pdf_cache = PDFCache(PDF_CACHE_SIZE)


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


def preprocess_png(png: bytes) -> bytes:
    with Image.open(io.BytesIO(png)) as im:
        im = im.convert("L")
        width, height = im.size
        max_edge = max(width, height)
        if max_edge > MAX_EDGE:
            scale = MAX_EDGE / max_edge
            im = im.resize((max(1, int(width * scale)), max(1, int(height * scale))), Image.Resampling.LANCZOS)
        out = io.BytesIO()
        im.save(out, format="PNG", optimize=True)
        return out.getvalue()


def render_pages(pdf_path: Path, page_numbers: list[int]) -> dict[int, bytes]:
    if not page_numbers:
        return {}
    wanted = set(page_numbers)
    first, last = min(page_numbers), max(page_numbers)
    with tempfile.TemporaryDirectory(prefix="docforge-ocr-") as td:
        prefix = Path(td) / "page"
        subprocess.run(
            [
                "pdftoppm",
                "-f",
                str(first),
                "-l",
                str(last),
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
        out: dict[int, bytes] = {}
        for png in sorted(Path(td).glob("page-*.png")):
            try:
                page_num = int(png.stem.rsplit("-", 1)[-1])
            except ValueError:
                continue
            if page_num in wanted:
                out[page_num] = png.read_bytes()
        missing = wanted - set(out.keys())
        if missing:
            raise RuntimeError(f"pdftoppm missing pages: {sorted(missing)}")
        return out


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


def ocr_tesseract(png: bytes, langs) -> tuple[str, float]:
    png = preprocess_png(png)
    with tempfile.TemporaryDirectory(prefix="docforge-tess-") as td:
        img = Path(td) / "page.png"
        img.write_bytes(png)
        out_base = Path(td) / "out"
        subprocess.run(
            [
                "tesseract",
                str(img),
                str(out_base),
                "-l",
                tess_langs(langs),
                "--oem",
                TESS_OEM,
                "--psm",
                TESS_PSM,
                "tsv",
            ],
            check=True,
            capture_output=True,
            timeout=TIMEOUT,
        )
        tsv = Path(str(out_base) + ".tsv").read_text(encoding="utf-8", errors="replace")
        return parse_tsv(tsv)


def _rapid_client():
    global _rapid_engine
    with _rapid_lock:
        if _rapid_engine is None:
            from rapidocr_onnxruntime import RapidOCR

            _rapid_engine = RapidOCR()
        return _rapid_engine


def ocr_rapid(png: bytes, _langs) -> tuple[str, float]:
    import numpy as np

    png = preprocess_png(png)
    img = np.array(Image.open(io.BytesIO(png)))
    result, _ = _rapid_client()(img)
    if not result:
        return "", 0.0
    words: list[str] = []
    confs: list[float] = []
    for item in result:
        if len(item) < 3:
            continue
        text = str(item[1]).strip()
        if not text:
            continue
        try:
            conf = float(item[2])
        except (TypeError, ValueError):
            conf = 0.0
        words.append(text)
        confs.append(max(0.0, min(1.0, conf)))
    avg = sum(confs) / len(confs) if confs else 0.0
    return " ".join(words), avg


def ocr_image(png: bytes, langs) -> tuple[str, float, str]:
    engine = OCR_ENGINE
    if engine == "auto":
        try:
            text, conf = ocr_rapid(png, langs)
            if text.strip():
                return text, conf, "rapidocr"
        except Exception:
            pass
        text, conf = ocr_tesseract(png, langs)
        return text, conf, "tesseract"
    if engine == "rapidocr":
        text, conf = ocr_rapid(png, langs)
        return text, conf, "rapidocr"
    text, conf = ocr_tesseract(png, langs)
    return text, conf, "tesseract"


def ocr_page_number(png: bytes, langs, page_number: int) -> dict[str, Any]:
    text, conf, provider = ocr_image(png, langs)
    detected = langs[0] if langs else "und"
    return {
        "page_number": page_number,
        "text": text,
        "confidence": round(conf, 4),
        "language": LANG_MAP.get(str(detected).lower(), str(detected)),
        "engine": provider,
    }


def ocr_batch(pdf_path: Path, page_numbers: list[int], langs) -> list[dict[str, Any]]:
    pages = sorted(set(page_numbers))
    images = render_pages(pdf_path, pages)
    results: list[dict[str, Any]] = []
    workers = min(MAX_CONCURRENT, max(1, len(pages)))
    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = {
            pool.submit(ocr_page_number, images[page_num], langs, page_num): page_num for page_num in pages
        }
        for fut in as_completed(futures):
            results.append(fut.result())
    results.sort(key=lambda item: int(item["page_number"]))
    return results


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
            self._json(
                200,
                {
                    "status": "ok",
                    "provider": "docforge-ocr",
                    "version": tess_version(),
                    "engine": OCR_ENGINE,
                    "max_concurrent": MAX_CONCURRENT,
                    "timeout_sec": TIMEOUT,
                    "dpi": DPI,
                },
            )
            return
        self._json(404, {"error": "not found"})

    def _read_json_body(self) -> tuple[dict | None, str | None]:
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            length = 0
        if length <= 0 or length > MAX_BODY:
            return None, "invalid body"
        try:
            return json.loads(self.rfile.read(length)), None
        except json.JSONDecodeError:
            return None, "invalid json"

    def _decode_pdf(self, req: dict) -> tuple[bytes | None, str | None]:
        pdf_b64 = req.get("pdf_base64") or req.get("payload_base64") or ""
        if not pdf_b64:
            return None, "pdf_base64 required"
        try:
            return base64.b64decode(pdf_b64), None
        except Exception:
            return None, "invalid pdf_base64"

    def do_POST(self) -> None:  # noqa: N802
        path = self.path.split("?", 1)[0]
        if path not in ("/v1/ocr", "/ocr", "/v1/ocr/batch", "/ocr/batch"):
            self._json(404, {"error": "not found"})
            return
        req, err = self._read_json_body()
        if err or req is None:
            self._json(400, {"error": err or "invalid body"})
            return
        langs = req.get("language") or ["eng"]
        if isinstance(langs, str):
            langs = [langs]
        pdf, err = self._decode_pdf(req)
        if err or pdf is None:
            self._json(400, {"error": err or "pdf_base64 required"})
            return
        document_id = str(req.get("document_id") or "")

        if path in ("/v1/ocr/batch", "/ocr/batch"):
            raw_pages = req.get("page_numbers") or req.get("pages") or []
            if not isinstance(raw_pages, list) or not raw_pages:
                self._json(400, {"error": "page_numbers required"})
                return
            page_numbers: list[int] = []
            for item in raw_pages:
                try:
                    page = int(item)
                except (TypeError, ValueError):
                    self._json(400, {"error": "invalid page_numbers"})
                    return
                if page < 1:
                    self._json(400, {"error": "page_numbers must be >= 1"})
                    return
                page_numbers.append(page)
            try:
                with _ocr_sem:
                    pdf_path = _pdf_cache.materialize(document_id, pdf)
                    pages = ocr_batch(pdf_path, page_numbers, langs)
            except subprocess.TimeoutExpired:
                self._json(504, {"error": f"ocr timed out after {TIMEOUT}s"})
                return
            except subprocess.CalledProcessError as exc:
                detail = (exc.stderr or exc.stdout or b"").decode("utf-8", errors="replace")[:500]
                self._json(502, {"error": "ocr failed", "detail": detail})
                return
            except Exception as exc:
                self._json(502, {"error": str(exc)})
                return
            self._json(
                200,
                {
                    "pages": pages,
                    "provider": "docforge-ocr",
                    "version": tess_version(),
                    "engine": OCR_ENGINE,
                },
            )
            return

        try:
            page = int(req.get("page_number") or 0)
        except (TypeError, ValueError):
            page = 0
        if page < 1:
            self._json(400, {"error": "page_number must be >= 1"})
            return
        try:
            with _ocr_sem:
                pdf_path = _pdf_cache.materialize(document_id, pdf)
                images = render_pages(pdf_path, [page])
                result = ocr_page_number(images[page], langs, page)
        except subprocess.TimeoutExpired:
            self._json(504, {"error": f"ocr timed out after {TIMEOUT}s"})
            return
        except subprocess.CalledProcessError as exc:
            detail = (exc.stderr or exc.stdout or b"").decode("utf-8", errors="replace")[:500]
            self._json(502, {"error": "ocr failed", "detail": detail})
            return
        except Exception as exc:
            self._json(502, {"error": str(exc)})
            return
        self._json(
            200,
            {
                "text": result["text"],
                "confidence": result["confidence"],
                "language": result["language"],
                "provider": result["engine"],
                "version": tess_version(),
                "page_number": page,
            },
        )


def main() -> None:
    host, _, port_s = ADDR.rpartition(":")
    if not host:
        host = "0.0.0.0"
    httpd = ThreadingHTTPServer((host, int(port_s)), Handler)
    print(
        f"[ocr] listening on {host}:{port_s} "
        f"(engine={OCR_ENGINE}, concurrent={MAX_CONCURRENT}, timeout={TIMEOUT}s, dpi={DPI})",
        flush=True,
    )
    httpd.serve_forever()


if __name__ == "__main__":
    main()
