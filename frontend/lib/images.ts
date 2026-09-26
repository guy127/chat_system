import { ApiError } from "@/lib/api";
import { MAX_IMAGE_BYTES, type ErrorCode, type UploadedImage } from "@/types/chat";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? "/api";
const MAX_SIDE = 2048;

export const ACCEPT_IMAGES = "image/jpeg,image/png,image/gif,image/webp,image/heic,image/heif";

export interface PreparedImage {
  blob: Blob;
  width: number;
  height: number;
  previewURL: string; // object URL; revoke when done
}

export class ImageError extends Error {
  constructor(public code: ErrorCode) {
    super(code);
  }
}

/**
 * Re-encodes a photo in the browser before upload: applies EXIF rotation,
 * scales the long side to 2048 px and drops all metadata (GPS location
 * included). GIFs are sent as they are so animations keep working; the
 * server strips metadata again either way.
 */
export async function prepareImage(file: File): Promise<PreparedImage> {
  if (file.size > MAX_IMAGE_BYTES) throw new ImageError("image_too_large");
  if (file.type && !file.type.startsWith("image/")) throw new ImageError("image_unsupported");

  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: "from-image" });
  } catch {
    throw new ImageError("image_unsupported"); // e.g. HEIC outside Safari
  }

  if (file.type === "image/gif") {
    const out = { blob: file, width: bitmap.width, height: bitmap.height, previewURL: URL.createObjectURL(file) };
    bitmap.close();
    return out;
  }

  const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height));
  const width = Math.round(bitmap.width * scale);
  const height = Math.round(bitmap.height * scale);
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new ImageError("image_unsupported");
  const keepAlpha = file.type === "image/png";
  if (!keepAlpha) {
    ctx.fillStyle = "#fff"; // JPEG has no transparency
    ctx.fillRect(0, 0, width, height);
  }
  ctx.drawImage(bitmap, 0, 0, width, height);
  bitmap.close();

  const blob = await new Promise<Blob | null>((resolve) =>
    canvas.toBlob(resolve, keepAlpha ? "image/png" : "image/jpeg", 0.85),
  );
  if (!blob) throw new ImageError("image_unsupported");
  if (blob.size > MAX_IMAGE_BYTES) throw new ImageError("image_too_large");
  return { blob, width, height, previewURL: URL.createObjectURL(blob) };
}

/** Uploads with XMLHttpRequest because fetch cannot report upload progress. */
export function uploadImage(
  roomId: string,
  jwt: string,
  blob: Blob,
  onProgress: (fraction: number) => void,
): Promise<UploadedImage> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_BASE}/rooms/${roomId}/images`);
    xhr.setRequestHeader("Authorization", `Bearer ${jwt}`);
    xhr.responseType = "json";
    xhr.upload.onprogress = (e) => e.lengthComputable && onProgress(e.loaded / e.total);
    xhr.onload = () => {
      if (xhr.status === 201) return resolve(xhr.response as UploadedImage);
      const err = (xhr.response as { error?: { code: ErrorCode; message: string } } | null)?.error;
      reject(new ApiError(xhr.status, err?.code ?? "internal", err?.message ?? xhr.statusText));
    };
    xhr.onerror = () => reject(new ApiError(0, "internal", "network error"));
    const form = new FormData();
    form.append(
      "image",
      blob,
      blob.type === "image/png" ? "image.png" : blob.type === "image/gif" ? "image.gif" : "image.jpg",
    );
    xhr.send(form);
  });
}

// Images need the Authorization header, which <img src> cannot send, so they
// are fetched once and shown from an object URL shared by every render.
const cache = new Map<string, Promise<string>>();

export function loadImageURL(roomId: string, imageId: string, jwt: string): Promise<string> {
  let p = cache.get(imageId);
  if (!p) {
    p = fetch(`${API_BASE}/rooms/${roomId}/images/${imageId}`, { headers: { Authorization: `Bearer ${jwt}` } }).then(
      async (res) => {
        if (!res.ok) throw new ApiError(res.status, "not_found", res.statusText);
        return URL.createObjectURL(await res.blob());
      },
    );
    p.catch(() => cache.delete(imageId)); // allow a retry later
    cache.set(imageId, p);
  }
  return p;
}

/** Lets the sender show their own image from the local copy instead of downloading it again. */
export function primeImageURL(imageId: string, url: string) {
  if (!cache.has(imageId)) cache.set(imageId, Promise.resolve(url));
}
