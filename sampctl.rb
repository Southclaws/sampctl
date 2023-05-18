# typed: false
# frozen_string_literal: true

# This file was generated by GoReleaser. DO NOT EDIT.
class Sampctl < Formula
  desc "The Swiss Army Knife of SA:MP - vital tools for any server owner or library maintainer."
  homepage "https://github.com/Southclaws/sampctl"
  version "1.11.3"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/Southclaws/sampctl/releases/download/1.11.3/sampctl_1.11.3_darwin_amd64.tar.gz"
      sha256 "7a35ab9990868f5caccfb7e50263e0063e2df7f5c29b631bfdefa13459d02f89"

      def install
        bin.install "sampctl"
      end
    end
  end

  on_linux do
    if Hardware::CPU.intel?
      url "https://github.com/Southclaws/sampctl/releases/download/1.11.3/sampctl_1.11.3_linux_amd64.tar.gz"
      sha256 "31407fa4e9113c2e7878c473bce14cc9939c34e6672f68abc2949dd2d0a3e27f"

      def install
        bin.install "sampctl"
      end
    end
  end
end