param(
    [Parameter(Mandatory = $true)] [string] $Path,
    [string] $Publisher = ''
)

$ErrorActionPreference = 'Stop'
$signature = Get-AuthenticodeSignature -LiteralPath $Path
if ($signature.Status -ne 'Valid' -or $signature.SignatureType -ne 'Authenticode') {
    throw "The file must have a valid embedded Authenticode signature ($($signature.Status)): $Path"
}
if ($null -eq $signature.TimeStamperCertificate) {
    throw "The Authenticode signature must be timestamped: $Path"
}
$certificate = $signature.SignerCertificate
if ($Publisher -and $certificate.Subject -cne $Publisher) {
    throw "The Authenticode signature does not match the expected publisher: $Path"
}
Write-Output "Verified Authenticode signature by $($certificate.Subject)"
