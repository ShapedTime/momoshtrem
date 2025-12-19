# PrettyTVCatalog - Frontend Style Guide

## Design Philosophy

Apple TV + Netflix inspired: **Cinematic, immersive, content-first**. The UI should disappear and let the media shine.

---

## Color Palette

```css
/* Background layers (darkest to lightest) */
--bg-base:      #000000;    /* True black - video backgrounds */
--bg-primary:   #0a0a0a;    /* Main app background */
--bg-elevated:  #141414;    /* Cards, modals, dropdowns */
--bg-hover:     #1f1f1f;    /* Hover states */
--bg-active:    #292929;    /* Active/pressed states */

/* Text */
--text-primary:   #ffffff;    /* Headings, important text */
--text-secondary: #b3b3b3;    /* Body text, descriptions */
--text-muted:     #666666;    /* Metadata, timestamps */
--text-disabled:  #404040;    /* Disabled states */

/* Accent colors */
--accent-red:     #e50914;    /* Primary action (Netflix red) */
--accent-blue:    #0071e3;    /* Links, secondary actions */
--accent-green:   #46d369;    /* Success, available */
--accent-yellow:  #f5a623;    /* Warnings, ratings */

/* Semantic */
--border:         #333333;    /* Subtle borders */
--focus-ring:     #0071e3;    /* Keyboard focus */
--overlay:        rgba(0, 0, 0, 0.7);  /* Modal overlays */
```

---

## Typography

```css
/* Font stack - system fonts for performance */
--font-sans: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto,
             'Helvetica Neue', Arial, sans-serif;

/* Scale */
--text-xs:   0.75rem;   /* 12px - badges, metadata */
--text-sm:   0.875rem;  /* 14px - secondary text */
--text-base: 1rem;      /* 16px - body text */
--text-lg:   1.125rem;  /* 18px - emphasized body */
--text-xl:   1.25rem;   /* 20px - card titles */
--text-2xl:  1.5rem;    /* 24px - section headers */
--text-3xl:  2rem;      /* 32px - page titles */
--text-4xl:  2.5rem;    /* 40px - hero titles (mobile) */
--text-5xl:  3.5rem;    /* 56px - hero titles (desktop) */

/* Weights */
--font-normal:   400;
--font-medium:   500;
--font-semibold: 600;
--font-bold:     700;

/* Line heights */
--leading-tight:  1.2;   /* Headings */
--leading-normal: 1.5;   /* Body text */
--leading-relaxed: 1.7;  /* Long-form text */
```

---

## Spacing System

Use 4px base unit. Consistent spacing creates visual rhythm.

```css
--space-1:  0.25rem;   /* 4px */
--space-2:  0.5rem;    /* 8px */
--space-3:  0.75rem;   /* 12px */
--space-4:  1rem;      /* 16px */
--space-5:  1.25rem;   /* 20px */
--space-6:  1.5rem;    /* 24px */
--space-8:  2rem;      /* 32px */
--space-10: 2.5rem;    /* 40px */
--space-12: 3rem;      /* 48px */
--space-16: 4rem;      /* 64px */
```

---

## Responsive Breakpoints

**Mobile-first approach.** Base styles are mobile, then enhance.

```css
/* Breakpoints */
--screen-sm:  640px;   /* Large phones, small tablets */
--screen-md:  768px;   /* Tablets */
--screen-lg:  1024px;  /* Small desktops, landscape tablets */
--screen-xl:  1280px;  /* Desktops */
--screen-2xl: 1536px;  /* Large desktops, TVs */

/* Tailwind classes */
sm:   /* >= 640px  */
md:   /* >= 768px  */
lg:   /* >= 1024px */
xl:   /* >= 1280px */
2xl:  /* >= 1536px */
```

---

## Component Patterns

### Media Cards

```
Mobile (< 640px):      2-3 cards per row, 140px width
Tablet (768px+):       4-5 cards per row, 160px width
Desktop (1024px+):     5-6 cards per row, 180px width
Large (1280px+):       6-7 cards per row, 200px width
```

```tsx
// Card aspect ratio: 2:3 (standard poster)
<div className="
  w-[140px] sm:w-[160px] lg:w-[180px] xl:w-[200px]
  aspect-[2/3]
  rounded-md overflow-hidden
  transition-transform duration-200
  hover:scale-105 hover:z-10
">
```

### Hero Banner

```
Mobile:   40vh height, text centered, smaller font
Tablet:   50vh height, text left-aligned
Desktop:  60vh height, max-height 700px
```

```tsx
<section className="
  relative h-[40vh] sm:h-[50vh] lg:h-[60vh] lg:max-h-[700px]
  bg-gradient-to-t from-bg-primary via-transparent to-transparent
">
```

### Carousels

```
Mobile:   Edge-to-edge, 16px padding, peek next card
Tablet+:  Side margins, 24px gap between cards
```

```tsx
<div className="
  flex gap-3 sm:gap-4 lg:gap-6
  overflow-x-auto snap-x snap-mandatory
  px-4 sm:px-6 lg:px-12
  scrollbar-hide
">
```

### Buttons

```
Mobile:   Full width buttons, 48px min height (touch target)
Desktop:  Auto width, 44px height
```

```tsx
// Primary button
<button className="
  w-full sm:w-auto
  h-12 sm:h-11
  px-6 sm:px-8
  bg-accent-red hover:bg-red-700
  text-white font-semibold
  rounded-md
  transition-colors
">

// Secondary button
<button className="
  w-full sm:w-auto
  h-12 sm:h-11
  px-6 sm:px-8
  bg-white/10 hover:bg-white/20
  text-white font-medium
  rounded-md border border-white/20
  transition-colors
">
```

---

## Layout Containers

```tsx
// Page container with responsive padding
<main className="
  px-4 sm:px-6 lg:px-12 xl:px-16
  py-6 sm:py-8 lg:py-12
  max-w-screen-2xl mx-auto
">

// Section with header
<section className="mb-8 sm:mb-10 lg:mb-12">
  <h2 className="
    text-xl sm:text-2xl font-semibold mb-4 sm:mb-6
  ">
    Trending Now
  </h2>
  {/* content */}
</section>
```

---

## Animation & Transitions

Keep animations subtle and purposeful. Performance matters.

```css
/* Durations */
--duration-fast:   150ms;   /* Hover states */
--duration-normal: 200ms;   /* Most transitions */
--duration-slow:   300ms;   /* Complex animations */

/* Easing */
--ease-default: cubic-bezier(0.4, 0, 0.2, 1);  /* ease-out */
--ease-bounce:  cubic-bezier(0.68, -0.55, 0.265, 1.55);
```

```tsx
// Standard hover transition
className="transition-all duration-200 ease-out"

// Card scale on hover
className="transition-transform duration-200 hover:scale-105"

// Fade in
className="animate-fade-in" // Define in tailwind.config.js
```

---

## Shadows & Elevation

Minimal shadows. Use opacity and background shifts instead.

```css
/* Card shadows - only on hover/focus */
--shadow-card: 0 8px 16px rgba(0, 0, 0, 0.4);
--shadow-modal: 0 25px 50px rgba(0, 0, 0, 0.5);
--shadow-dropdown: 0 4px 12px rgba(0, 0, 0, 0.3);
```

---

## Touch & Accessibility

### Touch Targets
- Minimum 44x44px on mobile
- Minimum 48px height for buttons on mobile
- Adequate spacing between interactive elements (8px minimum)

### Focus States
```tsx
// Visible focus ring for keyboard navigation
className="
  focus:outline-none
  focus-visible:ring-2
  focus-visible:ring-accent-blue
  focus-visible:ring-offset-2
  focus-visible:ring-offset-bg-primary
"
```

### Reduced Motion
```tsx
// Respect user preferences
className="motion-reduce:transition-none motion-reduce:animate-none"
```

---

## Image Handling

```tsx
// Always use Next.js Image with responsive sizes
<Image
  src={posterUrl}
  alt={title}
  fill
  sizes="(max-width: 640px) 140px, (max-width: 1024px) 160px, 200px"
  className="object-cover"
  placeholder="blur"
  blurDataURL={blurPlaceholder}
/>
```

### Image URLs (TMDB)
```
Poster:   https://image.tmdb.org/t/p/w500{path}
Backdrop: https://image.tmdb.org/t/p/w1280{path}
Profile:  https://image.tmdb.org/t/p/w185{path}
```

---

## Component Checklist

Every component should:

- [ ] Work on mobile (320px minimum width)
- [ ] Have hover states for desktop
- [ ] Have focus states for keyboard
- [ ] Use semantic HTML elements
- [ ] Include loading/skeleton state
- [ ] Handle empty/error states
- [ ] Respect reduced motion preferences
