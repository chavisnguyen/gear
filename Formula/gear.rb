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
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.1/gear-darwin-arm64.tar.gz"
      sha256 "ef9ad0d9a3670f9bad0eb09f37006fbbdd202158a1d4915d607824744293908a"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.1/gear-darwin-amd64.tar.gz"
      sha256 "acaa2078b7051b6e655e7797a572e63421ba4b37013f7d2b37d5609be2a6b54a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.1/gear-linux-arm64.tar.gz"
      sha256 "3158fb77fe55c3cf747e784c51ab350d439d981a22acd595fae3bd13f6552075"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.1/gear-linux-amd64.tar.gz"
      sha256 "5b43e90a23b6fa88b7913d4e3cc4bc8b97ea40c8e97a0bbc6f01f358bed91f89"
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
