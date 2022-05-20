const axios = require('axios');

const openAPI = axios.create({
  baseURL: `https://developer.myshoplaza.com/openapi/2020-07`
});

openAPI.interceptors.request.use((config) => {
  return config;
});

openAPI.interceptors.response.use(
  function (response) {
    return response;
  },
  function (error) {
    return Promise.reject(error);
  }
);

module.exports = openAPI;
