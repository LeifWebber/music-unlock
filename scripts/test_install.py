"""Installer contract tests with local fake release downloads, no network."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

INSTALLER = Path(__file__).resolve().with_name("install.sh")


@unittest.skipIf(os.name == "nt", "POSIX installer; Windows uses ZIP")
class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="music-unlock-installer-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.mock = self.root / "mock"
        self.assets = self.root / "assets"
        self.mock.mkdir()
        self.assets.mkdir()
        self.name = "music-unlock-v0.2.0-linux-amd64"
        binary = b'#!/bin/sh\nprintf "argc=%s\\n" "$#"\nprintf "argument=<%s>\\n" "$@"\n[ "${1-}" != fail ] || exit 7\n'
        archive = self.assets / (self.name + ".tar.gz")
        with tarfile.open(archive, "w:gz") as out:
            info = tarfile.TarInfo(self.name + "/unmus")
            info.size, info.mode = len(binary), 0o755
            out.addfile(info, io.BytesIO(binary))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.assets / "SHA256SUMS").write_text(f"{digest}  {archive.name}\n")
        curl = self.mock / "curl"
        curl.write_text(f"#!{sys.executable}\n" + '''import os,sys,shutil
from pathlib import Path
args=sys.argv[1:];url=next(x for x in args if x.startswith('https://'))
if url.endswith('/releases/latest'):
 print('https://github.com/LeifWebber/music-unlock/releases/tag/v0.2.0',end='')
else:
 shutil.copyfile(Path(os.environ['TEST_ASSETS'])/url.rsplit('/',1)[1],args[args.index('-o')+1])
''')
        curl.chmod(0o755)
        uname = self.mock / "uname"
        uname.write_text('#!/bin/sh\ncase "$1" in -s) echo Linux;; -m) echo x86_64;; esac\n')
        uname.chmod(0o755)
        self.env = {**os.environ, "PATH": str(self.mock) + os.pathsep + os.environ["PATH"],
                    "TEST_ASSETS": str(self.assets), "TMPDIR": str(self.root)}

    def invoke(self, *args):
        # Exercise the actual pipe-to-sh entry point rather than only sh FILE.
        return subprocess.run(["sh", "-s", "--", *args], input=INSTALLER.read_text(),
                              text=True, capture_output=True, env=self.env)

    def test_install_to_path_with_spaces(self):
        destination = self.root / "bin with spaces"
        result = self.invoke("--bin-dir", str(destination))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((destination / "unmus").is_file())
        self.assertFalse(list(self.root.glob("music-unlock.*")))

    def test_one_shot_arguments_and_exit_code(self):
        result = self.invoke("--version", "v0.2.0", "--run", "输入 音乐.mflac", "输出")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("argument=<输入 音乐.mflac>", result.stdout)
        self.assertEqual(self.invoke("--run", "fail").returncode, 7)
        self.assertFalse(list(self.root.glob("music-unlock.*")))

    def test_one_shot_automatic_mode_and_single_destination(self):
        automatic = self.invoke("--run")
        self.assertEqual(automatic.returncode, 0, automatic.stderr)
        self.assertIn("argc=0", automatic.stdout)
        destination = self.invoke("--run", "系统下载/已解锁音乐")
        self.assertEqual(destination.returncode, 0, destination.stderr)
        self.assertIn("argc=1", destination.stdout)
        self.assertIn("argument=<系统下载/已解锁音乐>", destination.stdout)
        self.assertFalse(list(self.root.glob("music-unlock.*")))

    def test_existing_command_is_preserved_unless_force_is_explicit(self):
        destination = self.root / "bin"
        destination.mkdir()
        binary = destination / "unmus"
        binary.write_text("another program")
        result = self.invoke("--bin-dir", str(destination))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(binary.read_text(), "another program")
        result = self.invoke("--bin-dir", str(destination), "--force")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(binary.read_text().startswith("#!/bin/sh"))

    def test_force_does_not_follow_existing_symlink(self):
        destination = self.root / "bin"
        destination.mkdir()
        unrelated = self.root / "other program"
        unrelated.write_text("keep me")
        (destination / "unmus").symlink_to(unrelated)
        result = self.invoke("--bin-dir", str(destination), "--force")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(unrelated.read_text(), "keep me")
        self.assertTrue((destination / "unmus").is_symlink())

    def test_checksum_failure_preserves_existing_installation(self):
        destination = self.root / "bin"
        destination.mkdir()
        binary = destination / "unmus"
        binary.write_text("keep me")
        (self.assets / "SHA256SUMS").write_text(f"{'0' * 64}  {self.name}.tar.gz\n")
        result = self.invoke("--bin-dir", str(destination))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(binary.read_text(), "keep me")
        self.assertIn("SHA256 verification failed", result.stderr)


if __name__ == "__main__":
    unittest.main()
