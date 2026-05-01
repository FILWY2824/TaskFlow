Add-Type -AssemblyName System.Drawing
$img = [System.Drawing.Image]::FromFile('icons/tray.png')
$bmp = New-Object System.Drawing.Bitmap($img.Width, $img.Height, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.DrawImage($img, 0, 0)
$img.Dispose()
$bmp.Save('icons/tray.png', [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
$g.Dispose()