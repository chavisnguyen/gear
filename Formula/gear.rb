class Gear < Formula
  desc "Declarative multi-device CLI tool hub over Homebrew, npm, and uv"
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
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.2/gear-darwin-arm64.tar.gz"
      sha256 "7df5cd3cad95f4c023a1a54f402a3d4dd59e38d5760230e08a04ee572492baa4"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.2/gear-darwin-amd64.tar.gz"
      sha256 "58cb9cb70e15f1acd6f699178f3b30121ff31d87e14a27da132bc48afb5abd54"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.2/gear-linux-arm64.tar.gz"
      sha256 "e49250b0dc4e6515a09f39c0b237390202ddda6f37a0a713b99b7a4d87f2aeca"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.2/gear-linux-amd64.tar.gz"
      sha256 "e5d04146cdcae19bbece076f3e53951d6fb8c4ae96fcb5d6a4082333104cd59e"
    end
  end

  def install
    bin.install "gear"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gear version")
  end
end
