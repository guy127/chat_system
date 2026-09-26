import Image from "next/image";

interface Props {
  size?: number;
  className?: string;
}

/** The smalltalk "St" speech-bubble emblem. */
export default function Logo({ size = 48, className }: Props) {
  return <Image className={className} src="/logo.png" alt="" width={size} height={size} preload />;
}
