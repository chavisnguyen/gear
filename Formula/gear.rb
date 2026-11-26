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
      sha256 "fb18f6ec2a9189f836fc651e8f6932fb4fd27f7551fcd4c42968f8b351dd1d75"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-darwin-amd64.tar.gz"
      sha256 "9d970a32b936d00264fc4a134342b85c220fbb8bc07a7f85bdab71eb87300926"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-linux-arm64.tar.gz"
      sha256 "e4e5e427c73da0fba9e13f7f98c30372598b0c90ffc016109b988cd2afa1d817"
    end
    on_intel do
      url "https://github.com/chavisnguyen/gear/releases/download/v0.1.0/gear-linux-amd64.tar.gz"
      sha256 "3222727c989597d0b4faf007cdbfa2c68f5ff578ded5528874b72349593a2bc3"
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
