const { hasBeenSetAnalytics, isEnabledAnalytics, setAnalyticsConfig, emptyAnalyticsData } = require('../analytics');
jest.mock('../index');

describe('db/analytics.js', () => {
  beforeEach(() => {
    emptyAnalyticsData();
  });

  it('get and set', () => {
    expect(hasBeenSetAnalytics('user_id')).toBe(false);
    expect(isEnabledAnalytics('user_id')).toBe(false);
    setAnalyticsConfig({
      user_id: 'user_id',
      enabled: 0
    });
    expect(hasBeenSetAnalytics('user_id')).toBe(true);
    expect(isEnabledAnalytics('user_id')).toBe(false);
  });
});
