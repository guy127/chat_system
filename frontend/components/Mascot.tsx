import { useId } from "react";

interface Props {
  size?: number;
  mood?: "happy" | "sad";
  className?: string;
}

/** Bubbly, the smalltalk speech-bubble mascot. */
export default function Mascot({ size = 48, mood = "happy", className }: Props) {
  const gradient = useId();
  return (
    <svg className={className} width={size} height={size} viewBox="0 0 48 48" aria-hidden="true">
      <defs>
        <linearGradient id={gradient} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#c084fc" />
          <stop offset="1" stopColor="#f472b6" />
        </linearGradient>
      </defs>
      <path
        d="M24 5c11 0 19 7.2 19 16.5S35 38 24 38c-2 0-3.9-.2-5.7-.7L10 42l1.6-7.4C7.6 31.6 5 26.9 5 21.5 5 12.2 13 5 24 5z"
        fill={`url(#${gradient})`}
      />
      <ellipse cx="18" cy="11.5" rx="5" ry="2.2" fill="#fff" opacity=".35" transform="rotate(-18 18 11.5)" />
      <circle cx="17.5" cy="20" r="2.6" fill="#2a1f3d" />
      <circle cx="30.5" cy="20" r="2.6" fill="#2a1f3d" />
      <circle cx="18.3" cy="19.1" r=".8" fill="#fff" />
      <circle cx="31.3" cy="19.1" r=".8" fill="#fff" />
      <ellipse cx="12.5" cy="25.5" rx="2.8" ry="1.7" fill="#fff" opacity=".45" />
      <ellipse cx="35.5" cy="25.5" rx="2.8" ry="1.7" fill="#fff" opacity=".45" />
      <path
        d={mood === "happy" ? "M20 25.5c2.2 2.4 5.8 2.4 8 0" : "M20.5 28c2-1.8 5-1.8 7 0"}
        stroke="#2a1f3d"
        strokeWidth="2.4"
        strokeLinecap="round"
        fill="none"
      />
    </svg>
  );
}
