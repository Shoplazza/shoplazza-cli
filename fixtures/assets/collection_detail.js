(function ($) {
  $.collectionDetail = function (params) {
    var collectionId = params.collection_id;
    var $section = $('[data-section-id=' + params.id + ']');
    var $content = $section.find('.collection-product__wrapper');
    var $number = $section.find('.collection__number');
    var isPaginationShow = params.is_pagination_show;
    var limit = params.limit; // 每页数量
    var total = params.pages; //总页数
    var queryParams = $.params();
    var page = parseInt(queryParams.page, 10) || 1, //分页码
      loading = false, // 请求数据标示
      hasMore = true; // 是否还有数据
    var sort = {
      name: 'Recommend',
      value: queryParams.sort_by || 'manual'
    };
    if (params.current_tags) {
      params.current_tags.split(',').map(function (i) {
        // TODO escape css selector
        $("[data-type=tag_filter] :radio[value='" + i.replace(/ /g, "\\ ") + "']").prop("checked", true)
      })
    }
    sort.value && ($(".collection__sort select").val(sort.value));
    queryParams.price && ($("[data-type=price_filter] :radio").each(function (i, r) {
      if (r.value.replace(/[^\d-]/g, "").replace("-", ",") == (queryParams.price || "").replace(/,$/, "")) {
        r.checked = true;
      }
    }));


    // 获取数据，拼接html模板
    var getData = function (param, reload, cb) {
      // 显示分页直接跳转页面
      if (isPaginationShow) {
        return location.href = "/collections/" + params.handle + (param.tags ? "/" : "") + (param.tags || "") + "?" + $.toQuery(Object.assign($.params(), param, {
          page: param.page + 1
        }));
      }
      // 请求数据
      $(document).trigger('dj.common.load_more.loading.start', { sectionId: params.id })
      $.get('/api/collections/' + encodeURIComponent(collectionId) + '/products?' + $.toQuery(param), function (res, status) {
        if (res && res.data && status == 'success') {
          var products = res.data.products || [];
          products = products.filter(function (item) {
            return item.published;
          })
          page++;
          hasMore = res.data.has_more == 1 ? true : false;
          var html = '';
          var topProductIds = [];
          $('.top-product').each(function(_, element) {
            topProductIds.push($(element).data('product-id'));
          });
          products = products.filter(function(p) {
            return topProductIds.indexOf(p.id) === -1
          });
          $('.collection-empty-art-tpl').remove();
          if (products.length > 0) {
            html = window.template('collection-art-tpl', {
              products: products,
              filter: params.filter,
            });
          } else if (res.data.count == 0 && !$('.top-product').length) {
            // 总数为0,提示搜索结果空
            html = window.template('collection-empty-art-tpl');
          }
          // 更新商品数量
          $number.html($number.text().replace(/\d+(\.\d+)?/g, res.data.count))
          // 如果reload，清空原来的列表，重置page数值
          reload && $content.find('.common__product-gap:not(.top-product)').remove() && (page = 1);
          $content.append(html);
        }else{
          hasMore = false;
        }
        if (hasMore) {
          $(document).trigger('dj.common.load_more.loading.end', { sectionId: params.id })
        } else {
          $(document).trigger('dj.common.load_more.hide', { sectionId: params.id })
        }
        loading = false;
      })
    }

    // 监听select排序，请求数据
    $(document).off('dj.common.sort_select.change').on('dj.common.sort_select.change', function (e, data) {
      $('#pagination').off('pagination-change');
      // 初始化组件
      if (isPaginationShow && !$.isMobile()) {
        initPagination(1);
      }
      // 请求数据
      if (data.sectionId == params.id) {
        hasMore = true;
        sort = data;
        getData({
          page: 0,
          sort_by: sort.value,
          limit: limit,
          tags: getTags('tag'),
          price: getTags('price'),
        }, true);
      }
    })

    var loadMore = function () {
      if (!loading && hasMore) {
        loading = true;
        getData({
          page: page,
          sort_by: sort.value,
          limit: limit,
          tags: getTags('tag'),
          price: getTags('price'),
        }, false);
      }
    }

    // 不分页，监听滚动，请求数据
    if (!isPaginationShow) {
      $(document).on('scroll', $.debounce(function () {
        // 判断是否到底
        if ($.isToPageEnd(params.id)) {
          loadMore();
        }
      }, 10, 50));
    }

    // 监听loadmore点击，请求数据
    $(document).on('dj.common.load_more.is_click', function (e, data) {
      if (data.sectionId == params.id) {
        loadMore();
      }
    })

    // filter(mobile)
    var filter_wrapper = $('.collection-filter__wrapper');
    $(document).on('click', '.collection__filter-by', function () {
      //$('html, body').addClass('collection-filter__scroll');
      filter_wrapper.addClass('collection-filter__wrapper-open');
    })
    var closeFilter = function () {
      //$('html, body').removeClass('collection-filter__scroll');
      filter_wrapper.removeClass('collection-filter__wrapper-open');
    }
    var getPrice = function(str) {
      var price = (str + '').replace(/[\r\n]/g, "").replace(/\ +/g, "");
      price = price.charAt(price.length - 2) == '.' ? price + '0' : price;
      var decimal = false; // 是否需要精度
      // 判断是否需要精度
      if (price.charAt(price.length - 3) == ',' || price.charAt(price.length - 3) == '.') {
        decimal = true;
        price = price.substr(0, price.length - 3) + '*' + price.substr(price.length - 3);
      }
      // 去掉千分位分割
      price = price.replace(/\./g, '').replace(/\,/g, '').replace(/\'/g, '');
      if (decimal) {
        price = price.replace('*', '.');
      }
      // 去掉货币符号
      var res = /\d+(\.\d+)?/g.exec(price.replace(/,/g, ''));
      return res && res[0] ? res[0] : price;
    }
    var getTags = function (type) {
      var tagList = [];
      $('.collection-filter__item[data-type=' + type + '_filter]').each(function (k, v) {
        var val = $(v).find('input:checked').val();
        if (type == 'price' && val) {
          var val_arr = val.split('-');
          if (val_arr.length == 2) {
            val = getPrice(val_arr[0]) + ',' + getPrice(val_arr[1]);
          } else if (val_arr.length == 1) {
            val = getPrice(val_arr[0]) + ',';
          }
        }
        val && tagList.push(val);
      })
      return tagList.join(',')
    }
    $(document).on('click', '.collection-filter__footer-confirm', closeFilter);
    filter_wrapper.click(function (e) {
      if ($(e.target).hasClass('collection-filter__wrapper')) {
        closeFilter();
      }
    });
    // clear all
    $('.collection-filter__wrapper').on('click', '.collection-filter-clear', function () {
      $('.custom-control-input').attr('checked', false);
      getData({
        page: 0,
        sort_by: sort.value,
        limit: limit,
        tags: getTags('tag'),
        price: getTags('price'),
      }, true);
    })
      .on('change', '.custom-control-input', function () {
        getData({
          page: 0,
          sort_by: sort.value,
          limit: limit,
          tags: getTags('tag'),
          price: getTags('price'),
        }, true);
      })
    if (window.SHOP_PARAMS.shop_env == '1') {
      $(window).resize($.throttle(closeFilter, 200, 200));
    }
    //分页
    var initPagination = function (page) {
      $("#pagination").pagination({
        labelPrev: '<' + params.lang.prev,
        labelNext: params.lang.next + '>',
        labelPageSize: params.lang.items_per_page,
        page: page,
        total: total,
        pageSize: limit,
        onChange: function (e, data) {
          getData({
            page: data.page - 1,
            sort_by: sort.value,
            limit: limit,
            tags: getTags('tag'),
            price: getTags('price'),
          }, true);
        }
      });
    }
    initPagination(page);
    $(document).on('dj.editor.update dj.editor.delete dj.editor.sort dj.editor.add', function () {
      $('body').removeClass('overflow-hidden');
    });
  }
})(window.jQuery);
