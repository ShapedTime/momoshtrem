// Quantise PR screenshots to a 256-colour palette (several times smaller).
// Run from web/: `node scripts/optimize-screenshots.mjs [dir]`.
import { readdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import sharp from 'sharp';

const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const root = path.resolve(web, process.argv[2] ?? '../docs/screenshots/v2');

let before = 0;
let after = 0;
for (const entry of await readdir(root, { recursive: true, withFileTypes: true })) {
  if (!entry.isFile() || !entry.name.endsWith('.png')) continue;
  const file = path.join(entry.parentPath, entry.name);
  const input = await readFile(file);
  const output = await sharp(input).png({ palette: true, quality: 90, compressionLevel: 9, effort: 8 }).toBuffer();
  before += input.length;
  if (output.length < input.length) {
    await writeFile(file, output);
    after += output.length;
  } else {
    after += input.length;
  }
}
console.log(`${(before / 1e6).toFixed(2)} MB -> ${(after / 1e6).toFixed(2)} MB`);
