"use client"

import api from "./client"
import type { User, UpdateUserRequest, ChangePasswordRequest } from "@/types/api/users"

export const usersApi = {
  get: (userId: string) =>
    api.get<User>(`/api/v1/users/${userId}`),

  update: (userId: string, data: UpdateUserRequest) =>
    api.patch<User>(`/api/v1/users/${userId}`, data),

  changePassword: (userId: string, data: ChangePasswordRequest) =>
    api.post<{ message: string }>(`/api/v1/users/${userId}/change-password`, data),
}
