(function ($) {
  $.search_card = function (params) {
    var $section = $('[data-section-id=' + params.id + ']');
    var searchUrl = '/api/search?';
    var page = 1, //分页码
      limit = 12, // 每页数量
      loading = false, // 请求数据标示
      hasMore = true, // 是否还有数据
      key = params.keyword; // 搜索关键词

    // 获取数据，拼接html模板
    var getData = function (param, reload, cb) {
      // 请求数据
      $(document).trigger('dj.common.load_more.loading.start', { sectionId: params.id });
      $.get(param, function (res) {
        if (res) {
          var products = res.data.products || [];
          products = products.filter(function (item) {
            return item.published;
          });
          page++;
          hasMore = res.data.has_more == 1 ? true : false;
          var html = window.template('search-art-tpl', {
            products: products,
          });
          var content = $section.find('.common__product-row');
          // 如果reload，清空原来的列表，重置page数值
          reload && content.empty() && (page = 1);
          content.append(html);
          // 更新数量和关键字的显示,根据count是否为0渲染不同dom
          if (key && res.data.total > 0) {
            $section.find('.search__result').html('<div class="search__result_not-empty">' + params.search_result + '</div>');
          }
          $section.find('.search__key').html('“' + decodeURIComponent(res.data.keyword) + '“');
          $section.find('.search__count').html(res.data.total);
        } else {
          hasMore = false;
          // console.log(res.msg);
        }
        if (hasMore) {
          $(document).trigger('dj.common.load_more.loading.end', { sectionId: params.id });
        } else {
          $(document).trigger('dj.common.load_more.hide', { sectionId: params.id });
        }
        loading = false;
      });
    };

    var loadMore = function () {
      if (!loading && hasMore) {
        loading = true;
        getData(
          searchUrl +
          $.toQuery({
            type: "product",
            page: page,
            keyword: key,
            limit: limit
          }),
          false
        );
      }
    };

    // 监听滚动，请求数据
    $(document).on(
      'scroll',
      $.throttle(
        function () {
          // 判断是否到底
          if ($.isToPageEnd(params.id)) {
            loadMore();
          }
        },
        10,
        50
      )
    );

    // 监听loadmore点击，请求数据
    $(document).on('dj.common.load_more.is_click', function (e, data) {
      if (data.sectionId == params.id) {
        loadMore();
      }
    });
  };
})(window.jQuery);