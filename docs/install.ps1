# radii5 installer - short URL entry point
# Usage: irm https://ohcass.github.io/radii5/install.ps1 | iex
$raw = "https://raw.githubusercontent.com/ohcass/radii5/main/scripts"
$file = if ($PSVersionTable.PSVersion.Major -le 5) { "$raw/install-ps5.ps1" } else { "$raw/install.ps1" }
# Strip leading U+FEFF so the first-line # comment still lexes under Invoke-Expression.
Invoke-Expression ((Invoke-RestMethod $file) -replace '^\uFEFF', '')
