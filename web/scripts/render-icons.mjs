// One-off: renders the brand PNGs from the design's logo SVGs.
// Run from web/: `node scripts/render-icons.mjs`. The outputs are committed;
// re-run only when docs/redesign/design-cinematic/assets/logo changes.
import { copyFile, readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import sharp from 'sharp';

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const logo = path.resolve(web, '../docs/redesign/design-cinematic/assets/logo');
const canvas = '#0b0d12';

async function png(svgFile, size, out) {
  const svg = await readFile(path.join(logo, svgFile));
  await sharp(svg, { density: 384 }).resize(size, size).png({ compressionLevel: 9 }).toFile(path.join(web, out));
  console.log(`${out} (${size}px)`);
}

async function openGraph(out) {
  const width = 1200;
  const height = 630;
  const glow = Buffer.from(`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}">
    <defs><radialGradient id="g" cx="0.5" cy="0.62" r="0.6">
      <stop offset="0" stop-color="#f2b33d" stop-opacity="0.22"/>
      <stop offset="1" stop-color="#f2b33d" stop-opacity="0"/>
    </radialGradient></defs>
    <rect width="100%" height="100%" fill="${canvas}"/>
    <rect width="100%" height="100%" fill="url(#g)"/>
  </svg>`);
  const lockup = await sharp(await readFile(path.join(logo, 'lockup.svg')), { density: 600 })
    .resize({ width: 760 })
    .png()
    .toBuffer();
  const { height: lockupHeight = 0 } = await sharp(lockup).metadata();
  await sharp(glow)
    .composite([{ input: lockup, left: Math.round((width - 760) / 2), top: Math.round((height - lockupHeight) / 2) }])
    .png({ compressionLevel: 9 })
    .toFile(path.join(web, out));
  console.log(`${out} (${width}×${height})`);
}

await copyFile(path.join(logo, 'favicon-32.svg'), path.join(web, 'src/app/icon.svg'));
console.log('src/app/icon.svg (copied)');
await png('app-icon-180.svg', 180, 'src/app/apple-icon.png');
await png('app-icon-1024.svg', 192, 'public/icons/icon-192.png');
await png('app-icon-1024.svg', 512, 'public/icons/icon-512.png');
await png('maskable-512.svg', 512, 'public/icons/maskable-512.png');
await openGraph('src/app/opengraph-image.png');
