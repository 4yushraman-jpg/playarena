import type { AxiosError } from "axios"

export interface ApiErrorBody {
  error: string
  fields?: Record<string, string>
  organizations?: Array<{ id: string; name: string; slug: string; role: string }>
  code?: string
}

export function extractApiError(err: unknown): string {
  if (!err) return "An unexpected error occurred."
  const axiosErr = err as AxiosError<ApiErrorBody>
  if (axiosErr.response?.data?.error) return axiosErr.response.data.error
  if (axiosErr.message) return axiosErr.message
  return "An unexpected error occurred."
}

export function extractFieldErrors(err: unknown): Record<string, string> {
  const axiosErr = err as AxiosError<ApiErrorBody>
  return axiosErr?.response?.data?.fields ?? {}
}

export function isValidationError(err: unknown): boolean {
  const axiosErr = err as AxiosError<ApiErrorBody>
  return axiosErr?.response?.status === 400 && !!axiosErr.response?.data?.fields
}

export function isOrgRequiredError(err: unknown): boolean {
  const axiosErr = err as AxiosError<ApiErrorBody>
  return axiosErr?.response?.status === 409
}

export function isNotFoundError(err: unknown): boolean {
  const axiosErr = err as AxiosError<ApiErrorBody>
  return axiosErr?.response?.status === 404
}

export function isForbiddenError(err: unknown): boolean {
  const axiosErr = err as AxiosError<ApiErrorBody>
  return axiosErr?.response?.status === 403
}
