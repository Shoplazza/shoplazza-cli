const fs = require('fs-extra');
const FormData = require('form-data');
const { getThemeFilenameTypeAndLocation } = require('../utils');
const openAPI = require('./index');

exports.getShopDetail = (config = {}) => {
  return openAPI.get('/shop', config);
};

exports.getThemes = () => {
  return openAPI.get('/themes');
};

exports.getThemeDetail = (theme) => {
  return openAPI.get(`/themes/${theme}`);
};

exports.getDefaultThemeDetail = () => {
  return openAPI.get(`/themes/default-theme`);
};

exports.pullTheme = (theme) => {
  return openAPI.get(`/themes/${theme}/download`, { responseType: 'stream' });
};

exports.pushTheme = ({ zipPath, name, version, merchant_theme_id, theme_id }) => {
  const form = new FormData();
  form.append('file', fs.createReadStream(zipPath));
  return openAPI.post('/themes/upload', form, {
    headers: form.getHeaders(),
    params: { name, version, merchant_theme_id, theme_id }
  });
};

exports.getPushTask = (taskId) => {
  return openAPI.get(`/themes/task/${taskId}`);
};

exports.publishTheme = (theme) => {
  return openAPI.patch(`/themes/${theme}/publish`);
};

exports.deleteTheme = (theme) => {
  return openAPI.delete(`/themes/${theme}`);
};

exports.getFileList = (theme) => {
  return openAPI.get(`/themes/${theme}/doctree`);
};

exports.deleteFile = (theme, filename) => {
  const { type, location } = getThemeFilenameTypeAndLocation(filename);
  return openAPI.delete(`/themes/${theme}/doc`, { params: { type, location } });
};

exports.updateFile = (theme, filename) => {
  const { type, location } = getThemeFilenameTypeAndLocation(filename);
  return openAPI.patch(`/themes/${theme}/doc`, {
    doc: {
      type,
      location,
      content: fs.readFileSync(filename, 'utf-8')
    }
  });
};

exports.addFile = (theme, filename) => {
  const { type, location } = getThemeFilenameTypeAndLocation(filename);
  return openAPI.post(`/themes/${theme}/doc`, {
    doc: {
      type,
      location,
      content: fs.readFileSync(filename, 'utf-8')
    }
  });
};
