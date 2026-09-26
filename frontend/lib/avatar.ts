// Friendly per-person colours and initials, derived from stable ids so everyone
// keeps the same look across devices and reloads.

// Curated pastel hues (pink, violet, sky, mint, butter, peach, lilac, teal).
const HUES = [330, 268, 205, 158, 45, 18, 292, 182];

export function hueOf(key: string): number {
  let h = 0;
  for (const c of key) h = (h * 31 + (c.codePointAt(0) ?? 0)) >>> 0;
  return HUES[h % HUES.length];
}

// Thai vowels written before the consonant; "เก่ง" should read "เก", not "เ".
const LEADING_VOWEL = /^[เแโใไ]$/;
const segmenter = typeof Intl !== "undefined" && "Segmenter" in Intl ? new Intl.Segmenter("th") : null;

export function initials(name: string): string {
  const text = name.trim();
  if (!text) return "?";
  const chars = segmenter ? [...segmenter.segment(text)].map((s) => s.segment) : [...text];
  const first = LEADING_VOWEL.test(chars[0]) && chars.length > 1 ? chars[0] + chars[1] : chars[0];
  return first.toUpperCase();
}
