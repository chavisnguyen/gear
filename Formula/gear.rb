class Gear < Formula
  desc "Lightweight multi-device CLI tool & version hub with SHA-256 lockfile"
  homepage "https://github.com/chavisnguyen/gear"
  license "MIT"

  # NOTE: `url`/`sha256` below are rewritten on every published Release by
  # the "Release assets" workflow (.github/workflows/release.yml). Do not edit
  # by hand. (`version` is inferred from the tag in the URL.)
  livecheck do
    url :homepage
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-darwin-arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-darwin-amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-linux-arm64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-linux-amd64.tar.gz"
      sha256 "0000000000000000000000000000000000000000000000000000000000000000"
    end
  end

  def install
    bin.install "gear"
  end

  def caveats
    <<~EOS
      Add ~/.gear/bin to your PATH so tools installed by gear are available everywhere:
        export PATH="$HOME/.gear/bin:$PATH"
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gear version")
  end
end
