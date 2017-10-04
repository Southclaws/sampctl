class Sampctl < Formula
  desc "A small utility for starting and managing SA:MP servers with better settings handling and crash resiliency."
  homepage "https://github.com/Southclaws/sampctl"
  url "https://github.com/Southclaws/sampctl/releases/download/1.2.6-R2/sampctl_1.2.6-R2_darwin_amd64.tar.gz"
  version "1.2.6-R2"
  sha256 "9bbb1885073e2038a5d6129f23e74112903f8f68fcc3c72baf488511019a2de3"

  def install
    bin.install "sampctl"
  end

  test do
    
  end
end
