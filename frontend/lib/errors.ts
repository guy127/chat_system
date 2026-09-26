import { ApiError } from "@/lib/api";
import type { ErrorCode } from "@/types/chat";

const messages: Record<ErrorCode, string> = {
  not_found: "ไม่พบข้อมูลที่ต้องการ",
  unauthorized: "สิทธิ์เข้าห้องหมดอายุ ขอลิงก์เชิญใหม่จากเจ้าของห้อง",
  forbidden: "คุณไม่มีสิทธิ์ทำรายการนี้",
  invalid_input: "ข้อมูลไม่ถูกต้อง",
  invite_invalid: "ลิงก์เชิญนี้ใช้ไม่ได้หรือถูกยกเลิกแล้ว",
  invite_expired: "ลิงก์เชิญนี้หมดอายุแล้ว ขอลิงก์ใหม่จากเจ้าของห้อง",
  invite_exhausted: "ลิงก์เชิญนี้ถูกใช้ครบจำนวนแล้ว",
  room_closed: "ห้องนี้ปิดแล้ว",
  room_full: "ห้องนี้เต็มแล้ว (สูงสุด 200 คน)",
  rate_limited: "ทำรายการถี่เกินไป กรุณารอสักครู่",
  banned: "คุณถูกแบนจากห้องนี้",
  kicked: "คุณถูกเชิญออกจากห้องนี้",
  message_too_long: "ข้อความยาวเกิน 2,000 ตัวอักษร",
  image_too_large: "รูปใหญ่เกิน 10 MB",
  image_unsupported: "รองรับเฉพาะรูป JPEG, PNG, GIF, WebP และ HEIC",
  internal: "เกิดข้อผิดพลาด กรุณาลองใหม่",
};

export function errorText(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 0) return "เชื่อมต่อเซิร์ฟเวอร์ไม่ได้ ตรวจสอบอินเทอร์เน็ตแล้วลองใหม่";
    return messages[err.code] ?? err.message;
  }
  return messages.internal;
}

export function codeText(code: ErrorCode): string {
  return messages[code] ?? messages.internal;
}
