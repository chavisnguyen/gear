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
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.0/gear-darwin-arm64.tar.gz"
      sha256 "b0a1a79f33ed56e8ae731856a95db8d526c98698983f6cda897e4dfb67262fc6"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.0/gear-darwin-amd64.tar.gz"
      sha256 "bde4749ed433288c0b149616bb4601ea9a47ef12b31365bc13386ac6923d3260"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.0/gear-linux-arm64.tar.gz"
      sha256 "c8bde4fde44601ed30431778416df92b3bca679a51d1f5fd66a7c3ed49166cc1"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.0/gear-linux-amd64.tar.gz"
      sha256 "a92151f29c92104ff84409be899d6baf9926556b6f375c959adab1e8305bf6b4"
    end
  end

  def install
    bin.install "gear"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gear version")
  end
end
