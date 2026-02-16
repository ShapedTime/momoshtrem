'use client';

import { useRef, useEffect, useCallback, useState } from 'react';
import type { PieceState, DebugFileInfo, DebugReaderInfo } from '@/types/torrent-debug';
import { PiecePriority, PRIORITY_LABELS } from '@/types/torrent-debug';

const CELL_SIZE = 6;
const GAP = 1;
const STEP = CELL_SIZE + GAP;

const COLORS: Record<string, string> = {
  complete: '#22c55e',
  now: '#ef4444',
  next: '#f97316',
  readahead: '#d97706',
  normal: '#2563eb',
  none: '#1c1c1c',
  reader: '#06b6d4',
};

interface PieceBitmapProps {
  pieces: PieceState[];
  files: DebugFileInfo[];
  readers: DebugReaderInfo[];
  pieceLength: number;
}

function getPieceColor(piece: PieceState): string {
  if (piece.complete) return COLORS.complete;
  switch (piece.priority) {
    case PiecePriority.Now: return COLORS.now;
    case PiecePriority.Next: return COLORS.next;
    case PiecePriority.Readahead: return COLORS.readahead;
    case PiecePriority.Normal: return COLORS.normal;
    default: return COLORS.none;
  }
}

export function PieceBitmap({ pieces, files, readers, pieceLength }: PieceBitmapProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [tooltip, setTooltip] = useState<{
    x: number;
    y: number;
    text: string;
  } | null>(null);
  const [cols, setCols] = useState(0);

  // Calculate columns based on container width
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const width = entry.contentRect.width;
        const newCols = Math.max(1, Math.floor((width + GAP) / STEP));
        setCols(newCols);
      }
    });

    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  // Draw canvas
  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || cols === 0 || pieces.length === 0) return;

    const rows = Math.ceil(pieces.length / cols);
    const width = cols * STEP - GAP;
    const height = rows * STEP - GAP;

    const dpr = window.devicePixelRatio || 1;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = `${width}px`;
    canvas.style.height = `${height}px`;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    // Draw pieces
    for (let i = 0; i < pieces.length; i++) {
      const col = i % cols;
      const row = Math.floor(i / cols);
      const x = col * STEP;
      const y = row * STEP;
      const piece = pieces[i];

      const color = getPieceColor(piece);

      if (piece.partial && !piece.complete) {
        // Partial: dimmer color with dot
        ctx.globalAlpha = 0.5;
        ctx.fillStyle = color;
        ctx.fillRect(x, y, CELL_SIZE, CELL_SIZE);
        ctx.globalAlpha = 1;
        // Dot indicator for partial
        ctx.fillStyle = '#fff';
        ctx.beginPath();
        ctx.arc(x + CELL_SIZE / 2, y + CELL_SIZE / 2, 1, 0, Math.PI * 2);
        ctx.fill();
      } else {
        ctx.globalAlpha = 1;
        ctx.fillStyle = color;
        ctx.fillRect(x, y, CELL_SIZE, CELL_SIZE);
      }
    }

    // Draw file boundary lines
    ctx.globalAlpha = 0.6;
    ctx.strokeStyle = '#666';
    ctx.lineWidth = 1;
    for (const file of files) {
      if (file.begin_piece === 0) continue;
      const row = Math.floor(file.begin_piece / cols);
      const y = row * STEP - 1;
      ctx.beginPath();
      ctx.moveTo(0, y);
      ctx.lineTo(width, y);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;

    // Draw reader position markers (cyan triangles)
    for (const reader of readers) {
      const pieceIdx = reader.position_piece;
      if (pieceIdx < 0 || pieceIdx >= pieces.length) continue;
      const col = pieceIdx % cols;
      const row = Math.floor(pieceIdx / cols);
      const x = col * STEP + CELL_SIZE / 2;
      const y = row * STEP - 2;

      ctx.fillStyle = COLORS.reader;
      ctx.beginPath();
      ctx.moveTo(x, y);
      ctx.lineTo(x - 3, y - 5);
      ctx.lineTo(x + 3, y - 5);
      ctx.closePath();
      ctx.fill();
    }
  }, [pieces, files, readers, cols]);

  // Hover tooltip
  const handleMouseMove = useCallback(
    (e: React.MouseEvent<HTMLCanvasElement>) => {
      const canvas = canvasRef.current;
      if (!canvas || cols === 0) return;

      const rect = canvas.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const y = e.clientY - rect.top;

      const col = Math.floor(x / STEP);
      const row = Math.floor(y / STEP);
      const idx = row * cols + col;

      if (idx < 0 || idx >= pieces.length) {
        setTooltip(null);
        return;
      }

      const piece = pieces[idx];
      const status = piece.complete
        ? 'Complete'
        : piece.partial
          ? 'Partial'
          : 'Missing';
      const priority = PRIORITY_LABELS[piece.priority] || `Unknown(${piece.priority})`;

      // Find which file this piece belongs to
      const file = files.find(
        (f) => idx >= f.begin_piece && idx <= f.end_piece
      );
      const fileName = file ? file.path.split('/').pop() : 'N/A';

      setTooltip({
        x: e.clientX,
        y: e.clientY,
        text: `Piece ${idx} | ${status} | Priority: ${priority} | File: ${fileName}`,
      });
    },
    [pieces, files, cols]
  );

  const handleMouseLeave = useCallback(() => setTooltip(null), []);

  if (pieces.length === 0) {
    return (
      <div className="bg-bg-elevated rounded-lg p-8 text-center text-text-muted">
        No piece data available
      </div>
    );
  }

  return (
    <div ref={containerRef} className="relative">
      <canvas
        ref={canvasRef}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
        className="block cursor-crosshair"
      />
      {tooltip && (
        <div
          className="fixed z-50 px-2 py-1 text-xs bg-black/90 text-white rounded shadow-lg pointer-events-none whitespace-nowrap"
          style={{ left: tooltip.x + 12, top: tooltip.y - 8 }}
        >
          {tooltip.text}
        </div>
      )}
    </div>
  );
}
