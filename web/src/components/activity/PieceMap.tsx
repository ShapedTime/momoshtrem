'use client';

import { type PointerEvent, useEffect, useMemo, useRef, useState } from 'react';
import type { PieceRun, TorrentDetailFile } from '@/lib/api/contract';
import { expandRuns, fileAtPiece, PIECE_LABEL, type PieceCell, pieceGrid } from '@/lib/activity/pieces';
import { formatNumber } from '@/lib/title/format';

/** The cell colours, from the tokens: missing, have, downloading (partial), prioritised. */
const TOKENS = ['--surface-3', '--st-ready', '--st-downloading', '--accent'] as const;

interface PieceMapProps {
  runs: readonly PieceRun[];
  total: number;
  pieceLength: number | null;
  files: readonly Pick<TorrentDetailFile, 'path' | 'size_bytes' | 'plays_as' | 'ignored'>[];
  /** "1,100 of 1,718 pieces downloaded", for screen readers. */
  label: string;
}

/**
 * One square per piece, drawn on a canvas (ported from the legacy
 * PieceBitmap: columns from the width, sharp on high-DPI screens), with the
 * piece, its file and its state under the pointer.
 */
export function PieceMap({ runs, total, pieceLength, files, label }: PieceMapProps) {
  const wrap = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [width, setWidth] = useState(0);
  const [tip, setTip] = useState<{ x: number; y: number; text: string } | null>(null);
  const cells = useMemo(() => expandRuns(runs, total), [runs, total]);

  useEffect(() => {
    const element = wrap.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => setWidth(Math.floor(entry.contentRect.width)));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    const element = canvas.current;
    const context = element?.getContext('2d');
    if (!element || !context || width === 0 || cells.length === 0) return;
    const { cell, step, cols, rows } = pieceGrid(width, cells.length);
    const dpr = window.devicePixelRatio || 1;
    element.width = Math.round(cols * step * dpr);
    element.height = Math.round(rows * step * dpr);
    element.style.width = `${cols * step}px`;
    element.style.height = `${rows * step}px`;
    context.setTransform(dpr, 0, 0, dpr, 0, 0);
    context.clearRect(0, 0, cols * step, rows * step);
    const styles = getComputedStyle(document.documentElement);
    const colours = TOKENS.map((token) => styles.getPropertyValue(token).trim());
    for (let i = 0; i < cells.length; i++) {
      context.fillStyle = colours[cells[i]];
      context.fillRect((i % cols) * step, Math.floor(i / cols) * step, cell, cell);
    }
  }, [cells, width]);

  const show = (event: PointerEvent<HTMLCanvasElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const { step, cols } = pieceGrid(width, cells.length);
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    const piece = Math.floor(y / step) * cols + Math.floor(x / step);
    if (piece < 0 || piece >= cells.length || x >= cols * step) {
      setTip(null);
      return;
    }
    const file = pieceLength ? fileAtPiece(files, pieceLength, piece) : null;
    const what = file ? (file.plays_as.map((p) => p.label).join(', ') || file.ignored?.label || file.path.split('/').pop()) : null;
    setTip({
      x: Math.max(0, Math.min(rect.width - 220, x - 110)),
      y: Math.max(0, y - 40),
      text: [`Piece ${formatNumber(piece)}`, what, PIECE_LABEL[cells[piece] as PieceCell]].filter(Boolean).join(' · '),
    });
  };

  return (
    <div ref={wrap} className="pm-wrap">
      <canvas ref={canvas} className="pm" role="img" aria-label={label} onPointerMove={show} onPointerDown={show} onPointerLeave={() => setTip(null)} />
      {tip && (
        <span className="pm-tip" style={{ left: tip.x, top: tip.y }} aria-hidden="true">
          {tip.text}
        </span>
      )}
    </div>
  );
}
