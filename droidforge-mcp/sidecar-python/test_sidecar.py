#!/usr/bin/env python3
"""
test_sidecar.py - Unit tests for Python sidecars (df_analyze_trace and df_verify_media).
"""

import hashlib
import json
import os
import subprocess
import sys
import tempfile
import unittest

from df_analyze_trace import analyze_trace
from df_verify_media import verify_media, compute_sha256


class TestSidecar(unittest.TestCase):
    def setUp(self):
        self.tmp_dir = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.tmp_dir.cleanup()

    def test_verify_media_bit_exact(self):
        file1 = os.path.join(self.tmp_dir.name, "sample1.bin")
        file2 = os.path.join(self.tmp_dir.name, "sample2.bin")
        file3 = os.path.join(self.tmp_dir.name, "sample3.bin")

        data = b"droidforge-fixture-test-payload-12345"
        with open(file1, "wb") as f:
            f.write(data)
        with open(file2, "wb") as f:
            f.write(data)
        with open(file3, "wb") as f:
            f.write(b"different-payload")

        sha256_val = hashlib.sha256(data).hexdigest()

        # Match against golden hash
        res1 = verify_media(file1, mode="bit_exact", golden_hash=sha256_val)
        self.assertTrue(res1["hash_match"])
        self.assertEqual(res1["output_sha256"], sha256_val)

        # Match against identical src
        res2 = verify_media(file2, src_path=file1, mode="bit_exact")
        self.assertTrue(res2["hash_match"])

        # Mismatch
        res3 = verify_media(file3, src_path=file1, mode="bit_exact")
        self.assertFalse(res3["hash_match"])

    def test_verify_media_quality_bounded(self):
        file1 = os.path.join(self.tmp_dir.name, "video1.mp4")
        file2 = os.path.join(self.tmp_dir.name, "video2.mp4")

        # Fake video container content with header
        with open(file1, "wb") as f:
            f.write(b"\x00\x00\x00\x20ftypisom\x00\x00\x02\x00isomiso2avc1mp41" + b"\x00" * 1024)
        with open(file2, "wb") as f:
            f.write(b"\x00\x00\x00\x20ftypisom\x00\x00\x02\x00isomiso2avc1mp41" + b"\x00" * 1024)

        res = verify_media(file2, src_path=file1, mode="quality_bounded", min_vmaf=80.0, min_ssim=0.90)
        self.assertTrue(res["hash_match"])
        self.assertGreaterEqual(res["ssim"], 0.90)
        self.assertGreaterEqual(res["vmaf"], 80.0)
        self.assertEqual(res["decode_errors"], 0)
        self.assertIn("codec", res["ffprobe_meta"])

    def test_analyze_trace_text_fallback(self):
        trace_file = os.path.join(self.tmp_dir.name, "test.perfetto-trace")
        trace_content = """
# tracer: nop
#
#           TASK-PID   CPU#    TIMESTAMP  FUNCTION
#              | |       |          |         |
   SurfaceFlinger-1000  [001] 1000.100: Choreographer#doFrame: dur=14.5
   SurfaceFlinger-1000  [001] 1000.116: Choreographer#doFrame: dur=15.8
   SurfaceFlinger-1000  [001] 1000.133: Choreographer#doFrame: dur=18.2
   com.example.app-2000 [002] 1000.140: mem.rss: size=134217728
   com.example.app-2000 [002] 1000.145: mem.swap: size=16777216
   lmkd-500             [000] 1000.150: MemoryLimiter:AnonSwap killed pid 2000
"""
        with open(trace_file, "w", encoding="utf-8") as f:
            f.write(trace_content)

        summary = analyze_trace(trace_file, pkg="com.example.app")
        self.assertGreater(summary["p95_frame_ms"], 0.0)
        self.assertGreater(summary["peak_rss_mb"], 0.0)
        self.assertGreater(summary["peak_anonswap_mb"], 0.0)
        self.assertEqual(summary["memory_limiter_events"], 1)

    def test_cli_execution_with_uv(self):
        sidecar_dir = os.path.dirname(os.path.abspath(__file__))
        media_file = os.path.join(self.tmp_dir.name, "test.bin")
        out_json = os.path.join(self.tmp_dir.name, "out.json")

        with open(media_file, "wb") as f:
            f.write(b"cli-test-data")

        # Test CLI via python directly
        cmd = [
            sys.executable,
            os.path.join(sidecar_dir, "df_verify_media.py"),
            "--dst", media_file,
            "--mode", "bit_exact",
            "--golden-hash", hashlib.sha256(b"cli-test-data").hexdigest(),
            "--out", out_json
        ]
        res = subprocess.run(cmd, capture_output=True, text=True)
        self.assertEqual(res.returncode, 0, f"CLI error: {res.stderr}")
        self.assertTrue(os.path.exists(out_json))

        with open(out_json, "r") as f:
            data = json.load(f)
        self.assertTrue(data["hash_match"])


if __name__ == "__main__":
    unittest.main()
