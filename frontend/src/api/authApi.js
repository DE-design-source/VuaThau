import axiosClient from './axiosClient'

// Both resolve with { success, data: { user, access_token, expires_in } }
const authApi = {
  register: ({ username, email, password }) =>
    axiosClient.post('/auth/register', { username, email, password }),

  login: ({ email, password }) => axiosClient.post('/auth/login', { email, password }),
}

export default authApi
