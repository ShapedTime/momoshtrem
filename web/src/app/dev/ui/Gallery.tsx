'use client';

import { type ReactNode, useState } from 'react';
import {
  Copy,
  Eye,
  EyeOff,
  LogOut,
  Ellipsis,
  Plus,
  RefreshCw,
  Search,
  Settings,
  SlidersHorizontal,
  Trash2,
  Tv,
  WifiOff,
} from 'lucide-react';
import { CatAvatar, CatEyes, EmptyCat, Lockup, Mark, MarkMono, Wordmark } from '@/components/brand';
import {
  Banner,
  Button,
  Chip,
  Confidence,
  ConfirmDialog,
  CopyField,
  EmptyState,
  FilterChip,
  IconButton,
  Input,
  Kbd,
  ListRow,
  Loader,
  Menu,
  MenuContent,
  MenuItem,
  MenuSeparator,
  MenuTrigger,
  Panel,
  PosterCard,
  PosterGrid,
  PosterSkeletons,
  Progress,
  Rail,
  RailItem,
  RingMeter,
  SearchInput,
  SegmentedControl,
  Select,
  Sheet,
  Skeleton,
  SkeletonText,
  Spinner,
  StatusPill,
  Steps,
  Switch,
  Tabs,
  Textarea,
  useConfirm,
  useToast,
  WideSkeletons,
} from '@/components/ui';
import { STATUS, type StatusKey } from '@/lib/status';

function Section({ id, title, subtitle, children }: { id: string; title: string; subtitle?: string; children: ReactNode }) {
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-5 border-t border-line-1 pt-8">
      <div className="flex flex-col gap-1">
        <h2 id={`${id}-h`} className="t-title-2">
          {title}
        </h2>
        {subtitle && <p className="t-small text-fg-3">{subtitle}</p>}
      </div>
      {children}
    </section>
  );
}

function Frame({ label, children, className }: { label: string; children: ReactNode; className?: string }) {
  return (
    <div className={`flex min-w-0 flex-col gap-3 rounded-lg bg-ink-0 p-4 ${className ?? ''}`}>
      <p className="text-xs text-fg-3">{label}</p>
      {children}
    </div>
  );
}

const SWATCHES = [
  ['--ink-0', 'ink-0'],
  ['--bg', 'bg / ink-1'],
  ['--surface-1', 'surface-1'],
  ['--surface-2', 'surface-2'],
  ['--surface-3', 'surface-3'],
  ['--surface-4', 'surface-4'],
  ['--accent', 'accent'],
  ['--st-ready', 'ready'],
  ['--st-downloading', 'downloading'],
  ['--st-missing', 'missing'],
  ['--st-review', 'review'],
  ['--st-watched', 'watched'],
  ['--st-upcoming', 'upcoming'],
  ['--st-warning', 'warning'],
  ['--st-error', 'error'],
] as const;

const TYPE_SCALE = [
  ['t-display', 'Display · Tonight’s pick'],
  ['t-title-1', 'Title 1 · Settings'],
  ['t-title-2', 'Title 2 · Up next'],
  ['t-title-3', 'Title 3 · Remove from library?'],
  ['t-body-l', 'Body L · Sign in to pick tonight’s watch.'],
  ['t-body', 'Body · The quick brown fox jumps over the lazy dog.'],
  ['t-small', 'Small · 2024 · Series · 4 seasons'],
  ['t-micro', 'Micro · Integrations'],
  ['t-mono', 'Frieren.Beyond.Journeys.End.S01E29.1080p.WEB-DL.mkv'],
] as const;

export function Gallery() {
  const { toast } = useToast();
  const confirm = useConfirm();
  const [sheet, setSheet] = useState<'none' | 'default' | 'narrow' | 'full'>('none');
  const [nested, setNested] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [switchOn, setSwitchOn] = useState(true);
  const [chips, setChips] = useState({ uhd: true, fhd: false, dv: false });
  const [scope, setScope] = useState<'episode' | 'season' | 'any'>('episode');
  const [season, setSeason] = useState<'1' | '2' | '0'>('1');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);

  return (
    <main id="main" className="page flex flex-col gap-10 py-10">
      <header className="flex flex-col gap-3">
        <p className="t-micro text-accent-text">Design system · dev only</p>
        <h1 className="t-display">UI gallery</h1>
        <p className="max-w-prose text-fg-2">
          Every primitive and state from HANDOFF §3, rendered with the production components. Resize to 390 and 1440
          to check both layouts.
        </p>
      </header>

      <Section id="brand" title="Brand & Noir" subtitle="Mark, wordmark, lockup, avatar, loader eyes and the empty-state cats.">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Frame label="Lockup">
            <Lockup height={32} />
          </Frame>
          <Frame label="Mark · mono · wordmark">
            <div className="flex items-center gap-4">
              <Mark size={40} />
              <MarkMono size={40} className="text-fg-2" />
              <Wordmark height={22} />
            </div>
          </Frame>
          <Frame label="Cat avatar · loader eyes">
            <div className="flex items-center gap-4">
              <CatAvatar size={40} />
              <CatEyes width={72} />
            </div>
          </Frame>
          <Frame label="Empty cats: sit · sleep · blank · peek">
            <div className="flex items-end gap-3 text-surface-4">
              <EmptyCat variant="sit" size={52} />
              <EmptyCat variant="sleep" size={52} />
              <EmptyCat variant="blank" size={52} />
              <EmptyCat variant="peek" size={64} />
            </div>
          </Frame>
        </div>
      </Section>

      <Section id="colour" title="Colour" subtitle="Ink surfaces, one amber accent, and status colours that always come with an icon and a word.">
        <div className="grid grid-cols-3 gap-3 sm:grid-cols-5 lg:grid-cols-8">
          {SWATCHES.map(([token, name]) => (
            <div key={token} className="flex flex-col gap-1.5">
              <div className="h-14 rounded-md border border-line-2" style={{ background: `var(${token})` }} />
              <span className="text-xs text-fg-2">{name}</span>
            </div>
          ))}
        </div>
      </Section>

      <Section id="type" title="Type" subtitle="Geist for UI and display, Geist Mono for release names, paths and hashes.">
        <div className="flex flex-col gap-3">
          {TYPE_SCALE.map(([cls, text]) => (
            <p key={cls} className={`${cls} wrap-any`}>
              {text}
            </p>
          ))}
        </div>
      </Section>

      <Section id="buttons" title="Buttons" subtitle="One primary per view. Never forced full width. Solid danger only inside confirm dialogs.">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="primary">Find a torrent</Button>
          <Button variant="secondary">Subtitles</Button>
          <Button variant="ghost">See all</Button>
          <Button variant="glass">
            <Tv size={18} aria-hidden="true" /> How to play
          </Button>
          <Button variant="ready">Ready in Infuse</Button>
          <Button variant="danger">Remove show</Button>
          <Button variant="danger-quiet">
            <Trash2 size={16} aria-hidden="true" /> Remove
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="primary" size="sm">
            Small
          </Button>
          <Button variant="primary">Medium</Button>
          <Button variant="primary" size="lg">
            Large
          </Button>
          <IconButton aria-label="More actions" variant="secondary">
            <Ellipsis size={18} aria-hidden="true" />
          </IconButton>
          <Button variant="primary" loading>
            Adding
          </Button>
          <Button variant="secondary" loading>
            Test connection
          </Button>
          <Button variant="primary" disabled>
            Disabled
          </Button>
          <Button
            variant="secondary"
            loading={loading}
            onClick={() => {
              setLoading(true);
              window.setTimeout(() => setLoading(false), 1400);
            }}
          >
            <RefreshCw size={16} aria-hidden="true" /> Click to load
          </Button>
        </div>
      </Section>

      <Section id="inputs" title="Inputs" subtitle="44 px tall, 16 px text (no iOS zoom), amber focus ring, red when invalid.">
        <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
          <Input label="Username" autoComplete="off" placeholder="teymur" help="The same one you use in Infuse." />
          <Input label="Monthly AI budget" defaultValue="-5" error="Enter an amount from $0 to $200." inputMode="decimal" />
          <SearchInput aria-label="Search titles" placeholder="Search or jump to…" />
          <Input
            label="Password"
            size="lg"
            type={showPassword ? 'text' : 'password'}
            autoComplete="off"
            defaultValue="correct horse"
            end={
              <IconButton
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                aria-pressed={showPassword}
                onClick={() => setShowPassword((value) => !value)}
              >
                {showPassword ? <EyeOff size={20} aria-hidden="true" /> : <Eye size={20} aria-hidden="true" />}
              </IconButton>
            }
          />
          <Select label="Preferred resolution" defaultValue="2160p">
            <option value="2160p">2160p</option>
            <option value="1080p">1080p</option>
            <option value="720p">720p</option>
          </Select>
          <Textarea label="Hint for the AI" placeholder="These are absolute episode numbers" />
          <div className="flex items-center gap-3">
            <Switch checked={switchOn} onCheckedChange={setSwitchOn} label="Auto-apply high-confidence mappings" />
            <span className="text-fg-2">Auto-apply high-confidence mappings</span>
          </div>
          <p className="text-fg-2">
            Open the palette with <Kbd>⌘K</Kbd> or <Kbd>/</Kbd>
          </p>
        </div>
      </Section>

      <Section id="chips" title="Chips, segmented control, tabs" subtitle="Filter chips always show a facet count; zero is dimmed but visible.">
        <div className="flex flex-wrap items-center gap-2">
          <Chip tone="hi">2160p</Chip>
          <Chip>WEB-DL</Chip>
          <Chip tone="accent">Dolby Vision</Chip>
          <Chip tone="pack">Season pack</Chip>
          <Chip tone="warn">CAM</Chip>
          <Chip tone="err">No seeders</Chip>
        </div>
        <div className="fchips">
          <FilterChip pressed={chips.uhd} count={12} onPressedChange={(on) => setChips((c) => ({ ...c, uhd: on }))}>
            2160p
          </FilterChip>
          <FilterChip pressed={chips.fhd} count={31} onPressedChange={(on) => setChips((c) => ({ ...c, fhd: on }))}>
            1080p
          </FilterChip>
          <FilterChip pressed={chips.dv} count={0} onPressedChange={(on) => setChips((c) => ({ ...c, dv: on }))}>
            Dolby Vision
          </FilterChip>
          <FilterChip pressed count={12} onRemove={() => toast({ tone: 'info', title: 'Removed the 2160p filter' })}>
            Removable
          </FilterChip>
          <FilterChip pressed={false} onOpenMenu={() => setSheet('narrow')}>
            <SlidersHorizontal size={16} aria-hidden="true" /> Filters
          </FilterChip>
        </div>
        <SegmentedControl
          aria-label="Search scope"
          value={scope}
          onChange={setScope}
          options={[
            { value: 'episode', label: 'Episode' },
            { value: 'season', label: 'Season pack' },
            { value: 'any', label: 'Any' },
          ]}
        />
        <Tabs
          aria-label="Seasons"
          value={season}
          onChange={setSeason}
          tabs={[
            { value: '1', label: 'Season 1', count: 28 },
            { value: '2', label: 'Season 2', count: 10 },
            { value: '0', label: 'Specials', count: 4 },
          ]}
        />
      </Section>

      <Section id="status" title="Status" subtitle="One vocabulary (lib/status.ts): every surface uses these words and icons.">
        <div className="flex flex-wrap gap-2">
          {(Object.keys(STATUS) as StatusKey[]).map((key) => (
            <StatusPill key={key} status={key} />
          ))}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <StatusPill status="downloading" label="64%" pct={64} />
          <StatusPill status="ready" size="lg" label="Ready in Infuse" />
          <StatusPill status="airs_on" label="Airs Fri" />
          <Confidence level="high" />
          <Confidence level="medium" />
          <Confidence level="low" />
          <Confidence level="none" />
        </div>
      </Section>

      <Section id="progress" title="Progress" subtitle="Determinate when we know how far along it is, indeterminate only for short waits, steps when the work has phases.">
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <Frame label="Bars">
            <div className="flex flex-col gap-4">
              <Progress label="Frieren S01E29–E38" value={64} />
              <Progress label="Streaming buffer" value={38} tone="accent" />
              <Progress label="Watched" value={68} tone="watched" />
              <Progress label="Searching 3 indexers" indeterminate />
            </div>
          </Frame>
          <Frame label="Rings">
            <div className="flex items-center gap-4">
              <RingMeter value={12} size={40} label="12%" />
              <RingMeter value={64} size={40} label="64%" />
              <RingMeter value={100} size={40} label="Done" />
            </div>
          </Frame>
          <Frame label="Steps">
            <Steps
              steps={[
                { label: 'Fetching metadata', detail: '13 files', state: 'done' },
                { label: 'Matching files', detail: '10 matched', state: 'done' },
                { label: 'Asking AI about the tricky ones', detail: '2 left', state: 'active' },
                { label: 'Ready to review', state: 'pending' },
              ]}
            />
          </Frame>
        </div>
        <Panel>
          <Loader title="Writing your picks…" detail="Reading your history · 2 of 5 shelves done" />
        </Panel>
      </Section>

      <Section id="media" title="Media, cards and rails" subtitle="Titles always visible, 2-line clamp; fluid grid columns, never fixed card widths.">
        <PosterGrid aria-label="Poster grid">
          <PosterCard href="#" title="Frieren: Beyond Journey’s End" subtitle="2023 · Series" badge={<StatusPill status="downloading" label="64%" pct={64} onArt />} />
          <PosterCard href="#" title="Severance" subtitle="2022 · Series" badge={<StatusPill status="ready" onArt />} />
          <PosterCard href="#" title="The Studio" subtitle="2025 · Series" watchedPct={40} />
          <PosterCard href="#" title="Pluribus" reason="Because you loved Severance: a slow-burn mystery with a big idea." />
          <PosterCard href="#" title="Bluey" subtitle="2018 · Series" badge={<StatusPill status="needs_review" label="Review" onArt />} />
          <PosterCard href="#" title="One Battle After Another" subtitle="2025 · 2h 42m" badge={<StatusPill status="needs_torrent" label="No torrent" onArt />} />
        </PosterGrid>
        <Rail title="Up next" subtitle="Rail with arrows on fine pointers" seeAllHref="#">
          {Array.from({ length: 12 }, (_, i) => (
            <RailItem key={i}>
              <PosterCard href="#" title={`Title ${i + 1}`} subtitle="2024" />
            </RailItem>
          ))}
        </Rail>
      </Section>

      <Section id="panels" title="Panels, rows and banners" subtitle="Banners carry one action at most.">
        <Panel as="ul" aria-label="Rows">
          <ListRow as="li" leading={<Settings size={20} className="text-fg-3" aria-hidden="true" />} title="Settings" description="Integrations, AI and playback" trailing={<StatusPill status="ok" />} />
          <ListRow as="li" leading={<Tv size={20} className="text-fg-3" aria-hidden="true" />} title="Infuse & WebDAV" description="WebDAV ready" />
        </Panel>
        <CopyField value="https://momoshtrem.duckdns.org:36911/" label="WebDAV address" hint="Paste it into Infuse › Add share › WebDAV" />
        <div className="flex flex-col gap-3">
          <Banner tone="error" title="Can’t reach momoshtrem" icon={WifiOff} action={<Button size="sm">Retry</Button>}>
            Showing what it said at 21:42. Infuse can still play ready titles.
          </Banner>
          <Banner tone="warning" title="2 indexers timed out" action={<Button size="sm">Retry them</Button>}>
            Showing 27 results from Nyaa. TorrentLeech and The Pirate Bay didn’t answer within 8 s.
          </Banner>
          <Banner tone="review" title="3 files need review">
            The rest are mapped.
          </Banner>
          <Banner tone="ready" title="Connection works" />
          <Banner tone="accent" title="Picked up where you left off" />
        </div>
      </Section>

      <Section id="loading" title="Skeletons and empty states" subtitle="Same box sizes as the content. Noir’s silhouette, one plain sentence, and the next step.">
        <PosterGrid aria-label="Loading posters">
          <PosterSkeletons count={6} />
        </PosterGrid>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <WideSkeletons count={3} />
        </div>
        <div className="flex items-center gap-4">
          <Skeleton shape="circle" width={44} height={44} />
          <SkeletonText lines={3} className="flex-1" />
          <Spinner label="Loading" />
        </div>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <Panel>
            <EmptyState
              cat="sit"
              title="Your library is empty"
              actions={
                <>
                  <Button variant="primary">Browse Discover</Button>
                  <Button variant="ghost">
                    <Search size={16} aria-hidden="true" /> Search
                  </Button>
                </>
              }
            >
              Add a movie or show and it lands here, then in Infuse once it’s downloaded.
            </EmptyState>
          </Panel>
          <Panel>
            <EmptyState cat="peek" title="No torrents match these filters">
              45 results are hidden by <strong className="text-fg-1">2160p</strong> and{' '}
              <strong className="text-fg-1">Dolby Vision</strong>.
            </EmptyState>
          </Panel>
          <Panel>
            <EmptyState cat="sleep" title="Up next needs Trakt" actions={<Button variant="primary">Connect Trakt</Button>}>
              Connect it once and momoshtrem knows which episode is next for every show you watch. Read-only.
            </EmptyState>
          </Panel>
        </div>
      </Section>

      <Section id="overlays" title="Sheets, dialogs, menus and toasts" subtitle="Bottom sheet on phone, right-hand panel on desktop. Esc closes the top layer; focus returns to the opener.">
        <div className="flex flex-wrap gap-3">
          <Button onClick={() => setSheet('default')}>Open sheet</Button>
          <Button onClick={() => setSheet('narrow')}>Narrow sheet</Button>
          <Button onClick={() => setSheet('full')}>Full-height sheet</Button>
          <Button variant="danger-quiet" onClick={() => setConfirmOpen(true)}>
            <Trash2 size={16} aria-hidden="true" /> Remove
          </Button>
          <Button
            onClick={async () => {
              const ok = await confirm({
                title: 'Remove this torrent?',
                description: 'Infuse will no longer be able to play Severance · Season 2. The show stays in your library.',
                confirmLabel: 'Remove torrent',
              });
              toast({ tone: 'info', title: ok ? 'Confirmed' : 'Cancelled' });
            }}
          >
            useConfirm()
          </Button>
          <Menu>
            <MenuTrigger asChild>
              <Button>
                <Ellipsis size={18} aria-hidden="true" /> Menu
              </Button>
            </MenuTrigger>
            <MenuContent>
              <MenuItem icon={<Settings size={18} aria-hidden="true" />}>Settings</MenuItem>
              <MenuItem icon={<Copy size={18} aria-hidden="true" />} hint="⌘C">
                Copy path
              </MenuItem>
              <MenuSeparator />
              <MenuItem danger icon={<LogOut size={18} aria-hidden="true" />}>
                Log out
              </MenuItem>
            </MenuContent>
          </Menu>
        </div>
        <div className="flex flex-wrap gap-3">
          <Button onClick={() => toast({ tone: 'ready', title: 'Added to library', body: 'The Good Place · 4 seasons, 53 episodes', action: { label: 'Find a torrent', onClick: () => undefined } })}>
            Show success
          </Button>
          <Button onClick={() => toast({ tone: 'info', icon: Trash2, title: 'Removed Bluey · Season 3', body: 'Undo within 10 seconds', action: { label: 'Undo', onClick: () => undefined } })}>
            Show info
          </Button>
          <Button onClick={() => toast({ tone: 'error', title: 'Couldn’t reach momoshtrem', body: 'Retrying in 10 s. Your changes are kept.', action: { label: 'Retry now', onClick: () => undefined } })}>
            Show error
          </Button>
          <Button onClick={() => toast({ tone: 'ready', sticky: true, title: 'Frieren S01E29–E38 are ready', body: 'They’ll appear in Infuse after its next library refresh.', action: { label: 'How to play', onClick: () => undefined } })}>
            Show “ready”
          </Button>
        </div>

        <Sheet
          open={sheet !== 'none'}
          onOpenChange={(open) => !open && setSheet('none')}
          size={sheet === 'none' ? 'default' : sheet}
          title={sheet === 'full' ? 'Find a torrent' : 'Play it in Infuse'}
          description={sheet === 'full' ? 'Frieren · S01E29' : undefined}
          head={sheet === 'full' ? <SearchInput aria-label="Torrent search" defaultValue="Frieren 29" /> : undefined}
          footer={
            <>
              <Button variant="secondary" onClick={() => setNested(true)}>
                <Plus size={16} aria-hidden="true" /> Open nested
              </Button>
              <Button variant="primary" className="ml-auto" onClick={() => setSheet('none')}>
                Done
              </Button>
            </>
          }
        >
          <ol className="howto-steps">
            <li>
              <span className="n">1</span>
              <div>
                <p className="font-semibold">Open Infuse on Apple TV</p>
                <p className="t-small text-fg-2">Settings › Shares › Add share › WebDAV</p>
              </div>
            </li>
            <li>
              <span className="n">2</span>
              <div>
                <p className="font-semibold">Use this address</p>
                <div className="copy-field mt-2">
                  <code className="t-mono">https://momoshtrem.duckdns.org:36911/</code>
                  <Button size="sm">
                    <Copy size={15} aria-hidden="true" /> Copy
                  </Button>
                </div>
              </div>
            </li>
            <li>
              <span className="n">3</span>
              <div>
                <p className="font-semibold">Find it under Library</p>
                <p className="t-small text-fg-2">New episodes appear after Infuse’s next library refresh.</p>
              </div>
            </li>
          </ol>
          {sheet === 'full' && (
            <div className="mt-6 flex flex-col gap-3">
              {Array.from({ length: 12 }, (_, i) => (
                <Skeleton key={i} height={96} />
              ))}
            </div>
          )}
        </Sheet>
        <Sheet open={nested} onOpenChange={setNested} nested size="narrow" title="Filters">
          <p className="text-fg-2">Nested sheets stack on top. Esc closes only this one.</p>
        </Sheet>
        <ConfirmDialog
          open={confirmOpen}
          onOpenChange={setConfirmOpen}
          title="Remove Severance from your library?"
          description="This removes the show and its 19 episode mappings. Downloaded data for its 2 torrents is deleted too. Your Trakt history isn’t touched."
          confirmLabel="Remove show"
          onConfirm={() => {
            setConfirmOpen(false);
            toast({ tone: 'info', icon: Trash2, title: 'Removed Severance', action: { label: 'Undo', onClick: () => undefined } });
          }}
        />
      </Section>
    </main>
  );
}
