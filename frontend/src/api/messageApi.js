import axiosClient from './axiosClient'

const messageApi = {
  send: ({ receiverId, content }) =>
    axiosClient.post('/messages', { receiver_id: receiverId, content }),

  /** Conversation between the current user and `userId`. */
  getConversation: (userId, { limit = 50, offset = 0 } = {}) =>
    axiosClient.get(`/messages/${userId}`, { params: { limit, offset } }),
}

export default messageApi
