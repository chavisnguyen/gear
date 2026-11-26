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
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.1/gear-darwin-arm64.tar.gz"
      sha256 "4b29e8c05adb88520e05cee02d4146a89c2dac468e27916091832b618e4da36b"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.1/gear-darwin-amd64.tar.gz"
      sha256 "a27abe810f0386c59db841ad74e59283c350e31c16b4586673aa40a0eb7f4fbe"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.1/gear-linux-arm64.tar.gz"
      sha256 "4ae12289d88b91d2f2caf272747a5ae2e78c5380ac51f9200dd0ae9f86ad6e4f"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.1/gear-linux-amd64.tar.gz"
      sha256 "db70e0a93730a24097e567739b6c50aae972f499b81df3b0b04bbe6be5fef6b8"
    end
  end

  def install
    bin.install "gear"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gear version")
  end
end
