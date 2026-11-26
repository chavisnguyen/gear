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
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.3/gear-darwin-arm64.tar.gz"
      sha256 "16fe141a26f65fba0bba44ef2de812da9921a9cca8dc5d3a17509ee384a7a4e5"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.3/gear-darwin-amd64.tar.gz"
      sha256 "cbc52669ba93d6c4d342cceacce68def375159925e47e3e997768c725e98cb41"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.3/gear-linux-arm64.tar.gz"
      sha256 "4d6d321ee92fa2485b481805d8c43d007ea77045184a0cc8fb428c1051efb90f"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.2.3/gear-linux-amd64.tar.gz"
      sha256 "d002577c115dfcb4fe84fc57d9b4c4c2b3627caf3eb6cdf58c5c3dac0d3c1ad2"
    end
  end

  def install
    bin.install "gear"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/gear version")
  end
end
