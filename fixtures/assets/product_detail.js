(function ($) {
  $.fn.product_detail = function (opt) {
    var $container = $("#product_detail_" + opt.product.id);
    var $slider = $container.find(".support-slick").eq(0);
    var $thumbs = $container.find('.product-image__thumbs-content');
    var $document = $(document);
    if (opt.product.images.length > 1) {
      $slider.on('init', function () {
        $(this).find('.swiper-slide').removeClass('hidden');
      }).on('beforeChange', function (event, slick, currentSlide, nextSlide) {
        $container.find(".product-image__swiper_bullets").html((nextSlide + 1) + " / " + opt.product.images.length);
      });
      var initSlider = function () {
        $slider.slick({
          lazyLoad: 'ondemand',
          slidesToShow: 1,
          arrows: false,
          adaptiveHeight: true,
          autoplay: false,
          dots: false,
          infinite: true,
          initialSlide: opt.initialSlide || 0,
          touchThreshold: 10,
          waitForAnimate: false,
          useTransform: true,
          rtl: document.documentElement.getAttribute("dir") == "rtl"
        }).on('beforeChange', function (event, slick, currentSlide, nextSlide) {
          $thumbs.find(".slick-current").removeClass("slick-slide slick-current").end().find("[data-thumb-idx=" + nextSlide + "]").addClass("slick-slide slick-current");
          var action = $slider.data("once_action");
          if (action) return;
          if (nextSlide == currentSlide) {
            action = "";
          } else if (nextSlide - currentSlide == 1) {
            action = "next";
          } else if (nextSlide - currentSlide == -1) {
            action = "prev";
          } else if (nextSlide == 0) {
            action = "next";
          } else if (nextSlide == $thumbs.children().length - 1) {
            action = "prev";
          }
          $slider.data("once_action", action);
          nextSlide !== currentSlide && slick.$slides[currentSlide].querySelectorAll('video').forEach(function (video) {
            !video.paused && video.pause();
          });
        }).on("afterChange", function (e, $slick, idx) {
          var $first = $thumbs.find(".product-image__thumbs-item:eq(0)");
          var size = $thumbs.children().length;
          /*$thumbs.find(".slick-current").removeClass("slick-slide slick-current").end().find("[data-thumb-idx=" + idx + "]").addClass("slick-slide slick-current");*/

          if ($slider.data("once_action")) {
            var action = $slider.data("once_action");
            // lazy load other thumb images
            $thumbs.find(".lazy-lazyload").addClass("lazyload");
            var invisibleThumbs = 0 - (parseInt($first.css("margin-left"), 10) / 80);
            // console.log(action, invisibleThumbs, idx);
            if (action == "next" && idx - invisibleThumbs == 6) {
              $first.css({ marginLeft: '-' + ((invisibleThumbs + 1) * 80) + 'px' });
            }
            if (action == "next" && idx == 0) {
              $first.css({ marginLeft: '0px' });
            }
            if (action == "prev" && idx == (invisibleThumbs - 1)) {
              $first.css({ marginLeft: '-' + ((invisibleThumbs - 1) * 80) + 'px' });
            }
            if (action == "prev" && idx == (size - 1)) {
              $first.css({ marginLeft: '-' + ((size - 6) * 80) + 'px' });
            }
            $slider.data("once_action", false);
            return;
          }
          var marginLeft = 0;
          if (idx < 6) {
            marginLeft = 0;
          } else if (idx > (size - 6) && size > 6) {
            marginLeft = (size - 6) * 80;
          } else {
            marginLeft = idx * 80;
          }
          $first.css({ marginLeft: '-' + marginLeft + 'px' });
        });
        $slider.find(".d-none").removeClass("d-none");
        $slider.find(".product-image__swiper_img").zoom();
        $thumbs.on("mouseenter", "[data-thumb-idx]", function (e) {
          var idx = e.currentTarget.getAttribute("data-thumb-idx");
          $thumbs.find(".slick-current").removeClass("slick-slide slick-current").end().find("[data-thumb-idx=" + idx + "]").addClass("slick-slide slick-current");
        });
        $thumbs.on("mouseenter", "[data-thumb-idx]", $.debounce(function (e) {
          var idx = e.currentTarget.getAttribute("data-thumb-idx");
          $slider.find(".slick-list,.slick-track").stop();
          $slider.data("once_action", "dummy"); // 不需要计算marginLeft
          $slider.slick("slickGoTo", idx, false);
        }, 300));
        $container.on("click", ".swiper-button-prev,.sep-loaded-slider__button-prev", function () {
          var size = $thumbs.children().length;
          var cur = $slider.slick('slickCurrentSlide');
          var newIdx = cur == 0 ? (size - 1) : (cur - 1);
          $slider.data("once_action", "prev").slick("slickGoTo", newIdx, cur == 0);
        });
        $container.on("click", ".swiper-button-next,.sep-loaded-slider__button-next", function () {
          var size = $thumbs.children().length;
          var cur = $slider.slick('slickCurrentSlide');
          var newIdx = (cur == (size - 1)) ? 0 : cur + 1;
          $slider.data("once_action", "next").slick("slickGoTo", newIdx, (cur == (size - 1)));
        });

        var isDetail = $slider.parents('[data-section-type="product_detail"]').length;
        // track
        if (isDetail) {
          $slider.on('beforeChange', function () {
            $('body').trigger('track', {eventName: 'product_image_slide'})
          });
          $slider.on('click', '.zoom-img', function(e){
            try {
              $(e.target).prev('.product-image__swiper_img').data("zoom-toggle") && $('body').trigger('track', {eventName: 'product_image_zoom'});
            } catch (error) {}
          })
          if ("ontouchstart" in document.body) {
            $slider.on("touchstart", function (e) {
              try {
                e.originalEvent.touches.length > 1 && $('body').trigger('track', {eventName: 'product_image_zoom'});
              } catch (error) {}
            })
          }
        }
      }
      // ajax slick should update until modal visible
      if (opt.ajax) {
        var timer = setInterval(function () {
          if ($('#product-select-modal').length && $('#product-select-modal').is(':visible')) {
            clearInterval(timer);
            initSlider();
          }
        }, 10);
      } else {
        initSlider();
      }
      // 去掉加载过程中的背景
      $container.find(".loading_bg").each(function () {
        if (this.complete) {
          $(this).removeClass("loading_bg");
        } else {
          $(this).one("load", function () {
            $(this).removeClass("loading_bg");
          })
        }
      });
    } else {
      $slider.find("[data-lazy]").each(function () {
        $(this).attr("src", $(this).attr("data-lazy"));
      })
    }
    var variants = opt.product.variants;
    var minPriceVariant = variants.reduce(function (prev, cur) { return parseFloat(prev.price) > parseFloat(cur.price) ? cur : prev }, variants[0]);
    var maxPriceVariant = variants.reduce(function (prev, cur) { return parseFloat(prev.price) < parseFloat(cur.price) ? cur : prev }, variants[0]);
    // 首次拿选择子商品必须从radio中过滤，js 加载前隐藏域的值还没初始化
    var selected = opt.product.options.reduce(function (prev, cur, i) { return prev.filter(function (v) { return v['option' + (i + 1)] == $("[name=option" + (i + 1) + "-" + opt.product.id + "]:checked").val() }) }, variants)[0] || {
      price: parseFloat(minPriceVariant.price),
      price_min: parseFloat(minPriceVariant.price),
      price_max: parseFloat(maxPriceVariant.price),
      compare_at_price: parseFloat(minPriceVariant.compare_at_price),
      off_ratio: parseFloat(minPriceVariant.off_ratio),
      sales: opt.product.sales,
      available_quantity: 999999,
      available: opt.product.available
    };
    $("#selected_variant_id_" + opt.product.id).val(selected.id || "");
    // 兼容
    $document.data("djproduct", {
      product: opt.product,
      selected: selected,
      selectedVariants: selected.id ? [selected] : opt.product.variants,
      qty: parseInt($("#product_quantity_" + opt.product.id).val(), 10),
      element: $container.selector
    });
    $document.trigger('dj.product.variants.change', $document.data('djproduct'));
    $container.data('djproduct', $document.data('djproduct'));

  }
  $(function () {
    var $document = $(document);
    var $body = $(document.body);

    var variantsChange = function () {
      // checked
      var productData = $(document).data("djproduct");
      var variants = productData.product.variants.slice();
      var options = productData.product.options;

      var $cb = $("#" + $(this).attr("for"));
      // checked -> unchecked
      if ($cb.prop("checked")) {
        setTimeout(function () {
          $cb.prop("checked", false);
        }, 10);
      }
      setTimeout(function () { // 等待checked状态变化
        var selectedOpts = [];
        $("#product_detail_" + productData.product.id).find('[name^=option]:checked, option[name^=option]:selected').each(function (i, r) {
          var indexes = r.id.split("-"); // ["option1","0"]
          var optIdx = parseInt(indexes[0].substr(6), 10) - 1;
          var valueIdx = parseInt(indexes[1], 10);
          selectedOpts.push(indexes[0]);
          variants = variants.filter(function (v) { return v[indexes[0]] == options[optIdx].values[valueIdx] });
        });
        $("#selected_variant_id_" + productData.product.id).val(selectedOpts.length == productData.product.options.length ? variants[0].id : "");
        // 重新计算radio disabled状态
        $("#product_detail_" + productData.product.id).find('input.product-info__variants_radio:not(:checked),option.product-info__variants_value:not(:selected)').each(function (i, r) {
          var indexes = r.id.split("-"); // ["option1","0",'id']
          var optIdx = parseInt(indexes[0].substr(6), 10) - 1;
          var valueIdx = parseInt(indexes[1], 10);
          var tmpVariant = $.extend(true, {}, variants[0]);
          tmpVariant[indexes[0]] = options[optIdx].values[valueIdx];
          var tmpSelectedOpts = selectedOpts.slice();
          tmpSelectedOpts.indexOf(indexes[0]) == -1 && (tmpSelectedOpts.push(indexes[0]));
          var disabled = productData.product.variants.filter(function (v) {
            return (v.option1 == tmpVariant.option1 || tmpSelectedOpts.indexOf('option1') === -1) && (v.option2 == tmpVariant.option2 || tmpSelectedOpts.indexOf('option2') === -1) && (v.option3 == tmpVariant.option3 || tmpSelectedOpts.indexOf('option3') === -1) && v.available_quantity > 0;
          }).length == 0;
          $(r).prop("disabled", disabled);
          $(r).html($(r).val());
          $(r).parent('select').val() && disabled && $(r).html($(r).val() + ' (' + $(r).data('soldout') + ')');
          $(r).parent('select').val() && $(r).parent('select').removeClass('tw-text-black tw-opacity-50')
        })
        var minPriceVariant = variants.reduce(function (prev, cur) { return parseFloat(prev.price) > parseFloat(cur.price) ? cur : prev }, variants[0]);
        var maxPriceVariant = variants.reduce(function (prev, cur) { return parseFloat(prev.price) < parseFloat(cur.price) ? cur : prev }, variants[0]);
        productData.selected = (selectedOpts.length == productData.product.options.length ? variants[0] : {
          price: parseFloat(minPriceVariant.price),
          price_min: parseFloat(minPriceVariant.price),
          price_max: parseFloat(maxPriceVariant.price),
          compare_at_price: parseFloat(minPriceVariant.compare_at_price),
          off_ratio: parseFloat(minPriceVariant.off_ratio),
          sales: productData.product.sales,
          available_quantity: 999999,
          available: productData.product.available
        });
        productData.selectedVariants = variants;
        productData.qty = Math.min(parseInt($("#product_quantity_" + productData.product.id).val(), 10), productData.selected.available_quantity);
        $(document).data("djproduct", productData);
        $(document).trigger('dj.product.variants.change', productData);
        // 色卡label
        !$cb.prop('disabled') && $cb.parents('.product-info__variants_items').find('#variant_color-label').html($cb.prop("checked") ? ' - ' + $cb.val() : '');

        if (productData.selectedVariants.length === 1) {
          // quickview 中不更新url
          if ($('#product-select-modal.show').length) return;
          window.history.replaceState(null, '', '?' + $.toQuery($.extend($.params(), {
            variant: productData.selectedVariants[0].id
          })))
        }
      }, 20);
    }
    //子商品切换
    $body.on("click", ".product-info__variants_value label", function () {
      variantsChange.call(this);
    }).on('change', '.product-info__variants_items select', function () {
      variantsChange.call(this);
    });
    // +/-
    $body.on('click', "[data-click=decrease],[data-click=increase]", function (e) {
      var t = $(e.target).attr("data-click");
      var productData = $document.data("djproduct");
      productData.qty = productData.qty || 1;
      productData.qty += (t == "decrease" ? -1 : 1);
      $document.data("djproduct", productData);
      $document.trigger('dj.product.variants.change', $document.data('djproduct'));
    })
    // input product_quantity
    $body.on('blur', ".product-info__qty_num", function (e) {
      var productData = $document.data("djproduct");
      var correctedValue = Math.max(Math.min(parseInt($(this).val().replace(/\D/g, "").replace(/^0*/, ""), 10) || 0, productData.selected.available_quantity || productData.product.available_quantity), 1);
      productData.qty = parseInt(correctedValue, 10);
      $document.data("djproduct", productData);
      $document.trigger('dj.product.variants.change', $document.data('djproduct'));

    })

    // slider switch
    $document.on('dj.product.variants.change', function (e, data) {
      if (!data.selected.image) return;
      var $container = $("#product_detail_" + data.product.id);
      // 兼容公共卡片
      if (!$container.data('life-style')) return;

      var $slider = $container.find('.support-slick').eq(0);
      var idx = data.product.images.findIndex(function (row) {
        return row.src == data.selected.image.src;
      });

      $slider.hasClass('slick-initialized') && $slider.slick('slickGoTo', idx);
    });

    // update discount label
    $document.on('dj.product.variants.change', function (e, data) {
      // 更新折扣标签和优惠金额
      var off = window.SHOP_PARAMS.product_lang.off.replace(/\{\s*count\s*\}/, data.selected.off_ratio)
      var $productImage = $("#product_detail_" + data.product.id + ' .product-image');
      if (data.selected.off_ratio > 0 && data.selected.available) {
        var save_html = window.SHOP_PARAMS.product_lang.save_html.replace(/\{\s*saved_amount\s*\}/, (data.selected.compare_at_price - data.selected.price).toFixed(2))
        $productImage.find('.product-info__discount-label').html(off).end().find('.product-info__save-label').html(save_html);
        $productImage.find('.product-info__label').css('opacity', 1);
      } else {
        $productImage.find('.product-info__label').css('opacity', 0);
      }
    });

    // qty section sku
    $document.on('dj.product.variants.change', function (e, data) {
      $("#product_detail_" + data.product.id + " .product-info__qty_container").html(template("product-info-qty-tpl", { product: $.extend({}, data.product, data.selected, { id: data.product.id }), qty: data.qty }));
      $("#product_detail_" + data.product.id + " .product-info__qty_stock")[data.selectedVariants.length > 1 ? "hide" : "show"]();
      $("#product_detail_" + data.product.id + " .product-info__header-sku")[data.selectedVariants.length > 1 ? "hide" : "show"]().html(data.selected.sku);
    });

    // price section
    $document.on('dj.product.variants.change', function (e, data) {
      $("#product_detail_" + data.product.id + " .product-info__header_price-wrapper").html(template("product-info-price-wrapper", {
        product: data.selected
      }));
    });

    //描述 tab
    $(function(){
      $(document).off('change.descTab').on('change.descTab', '.product-info__desc-tab-cb', function(){
        var checked = $(this).prop('checked');
        var $content = $(this).next();
        $(this).parents('.product-info__desc-wrap').toggleClass('is-open', checked);
        checked
          ? $content.slideDown(300)
          : $content.slideUp(300);
      })
    })

    // 获取自定义参数
    function getProperties() {
      var productData = $document.data("djproduct");
      var properties = {};
      var productId = productData.product.id;
      var $properties = $(".product-info-" + productId);
      var formdata = $properties.serializeArray();
      for (var i = 0; i < formdata.length; i++) {
        var result = /properties\[(.+)\]/g.exec(formdata[i].name);
        if (result && result[1]) {
          properties[result[1]] = formdata[i].value;
        }
      }
      var items = $properties.find('.required');
      var valid = true;
      for (i = 0; i < items.length; i++) {
        var $parent = $(items[i]).parents('.line-item-property__field');
        $parent.find('.not-empty').remove();
        if ($(items[i]).val() == '') {
          $(items[i]).addClass('not-empty-field');
          $parent.append('<div class="not-empty" style="color: #ea0000;">can not be empty</div>');
          valid = false;
        } else {
          $parent.removeClass('not-empty-field');
        }
      }
      return valid ? properties : false;
    }
    // add to cart
    $(document).on("dj.common.product.atc", function (e, options) {
      var properties = options.properties || {};
      $.loading.show();
      $.post("/api/cart", {
        product_id: options.product_id,
        variant_id: options.variant_id,
        quantity: options.quantity,
        properties: properties,
      }).always(function (res) {
        var data = res.readyState ? res.responseJSON : res;
        //addToCart按钮可以选择流程：1）to_cart: 跳到购物车 2）to_checkout: 跳到支付页 3）to_toast:弹提示框，页面不跳转
        var process = options.process;
        if (data.state === "success") {
          $(document.body).trigger("dj.addToCart", {
            id: options.product_id,
            number: options.quantity,
            childrenId: options.variant.id,
            item_price: options.variant.price,
            name: options.product.title,
            type: options.variant.type ? options.variant.type : options.product.type,
            properties: properties,
            quantity: options.quantity,
            variant_id: options.variant.id,
            product_id: options.product_id,
            product: options.product,
            variant: options.variant,
            source: options.source,
            process: options.process
          })

          if (process == 'to_cart') {
            $.loading.hide();
            location.href = '/cart';
            return void 0;
          } else if (process == 'to_checkout') {
            $.loading.hide();
            $(document).trigger("dj.common.product.buy_now", options);
          } else if (process == 'toast') {
            $.loading.hide();
            $.toast.show({ content: window.SHOP_PARAMS.product_lang.added_to_cart_successfully });
          } else if (process == 'to_toast') {
            // 购物车弹窗
            if (window.SHOP_PARAMS.template_type == '13') { $.loading.hide(); }
          }
          $(document).trigger('dj.common.cart.change')
        } else {
          $.loading.hide();
          $.toast.show({ content: data.message || 'Unknown error', type: 'error' });
        }
      })

    })
    $body.on("click", "[data-click=addToCart]", function (e) {
      var properties = getProperties();
      if (!properties) return;
      var productData = $document.data("djproduct");
      var values = $.params("?" + $(e.target).parents(".product-info").serialize());
      if (!values.variant_id) {
        $.toast.show({ type: 'error', content: window.SHOP_PARAMS.product_lang.select_variant });
        return;
      }
      values.properties = properties;
      values.process = (window.SHOP_PARAMS.product_settings || {}).add_to_cart_process;
      values.product = productData.product;
      values.variant = productData.selected;
      values.quantity = productData.qty || 1;
      $(document).trigger("dj.common.product.atc", values);
    })
    // buy now
    $(document).on("dj.common.product.buy_now", function (e, options) {
      var properties = options.properties || {};
      $.loading.show();
      $.post('/api/checkout/order', {
        line_items: [{
          quantity: options.quantity,
          variant_id: options.variant_id,
          note: '',
          properties: properties,
        }],
        refer_info: {
          source: 'buy_now'
        }
      }, function (ret) {
        $.loading.hide();
        if (ret.state === 'success') {
          return (location.href = '/checkout/' + ret.data.order_token + '?step=contact_information');
        } else {
          $.toast.show({
            content:
              {
                '30003': "Some items have been removed, please re-order",
                'line_items_variant_withdraw': "Some items have been sold out, please re-order",
                'line_items_variant_sold_out': "Some items are not in stock, please re-order"
              }[ret.state] || ret.errors[0] || "Some items information has been updated, please re-order"
          });
        }

      });

    })
    $body.on("click", "[data-click=submit]", function (e) {
      var values = $.params("?" + $(e.target).parents(".product-info").serialize());
      if (!values.variant_id) {
        $.toast.show({ type: 'error', content: window.SHOP_PARAMS.product_lang.select_variant });
        return;
      }
      var properties = getProperties();
      if (!properties) return;
      values.properties = properties;
      $(document).trigger("dj.common.product.buy_now", values);
    })

    // quickview
    $(document).on('dj.common.product.select', function (e, options) {
      // quick view弹窗
      var originData = $(document).data('djproduct');
      $(document).on('hide.bs.modal', '#product-select-modal', function () {
        $(document).data('djproduct', originData);
      });
      $.loading.show();
      var is_select_default_variants = window.SHOP_PARAMS.is_select_default_variants || false;
      $.get("/api/products/" + options.id, function (res) {
        $.loading.hide();
        $('#product-select-modal').remove();
        var product = res && res.data && res.data.product;
        if (!product) return;
        var selectedVariantId = "";
        var selectedVariant = "";
        var priceMin = 99999999;
        var comparePriceMin = 99999999;
        var priceMax = 0;
        if (is_select_default_variants || product.variants.size == 1) {
          for (var i = 0; i < product.variants.length; i++) {
            var variant = product.variants[i];
            if (variant.available_quantity > 0) {
              selectedVariantId = variant.id;
            break;
            }
          }
        }
        for (var i = 0; i < product.variants.length; i++) {
          var variant = product.variants[i];
          if (variant.compare_at_price < comparePriceMin) {
            comparePriceMin = variant.compare_at_price;
          }
          if (variant.price < priceMin) {
            priceMin = variant.price;
          }
          if (variant.price > priceMax) {
            priceMax = variant.price;
          }
          if (variant.id == selectedVariantId) {
            selectedVariant = variant;
          }
        }
        var initialSlide = 0;
        if (selectedVariant && selectedVariant.image && selectedVariant.image.src) {
          for (var i = 0; i < product.images.length; i++) {
            if (product.images[i] == selectedVariant.image.src ) {
              initialSlide = i;
            }
          }
        }
        $('body').prepend(template('product-select-wrapper', {product: product, initialSlide: initialSlide, selectedVariantId: selectedVariantId, selectedVariant: selectedVariant, priceMin: +priceMin, comparePriceMin: +comparePriceMin, priceMax: +priceMax}));
        if (window.innerWidth <= 768) {
          $('#product-select-modal').removeClass("fade");
        }
        $('#product-select-modal').modal('show');
        $("#product-select-modal .product-detail").product_detail({product: product, initialSlide: initialSlide, ajax: true });
        $(document).trigger('plugin_currency_update');
      });
      e.stopPropagation();
      e.preventDefault();
      return false;
    });
    // quick view  detail url
    $document.on('dj.product.variants.change', function (e, data) {
      var selectedVariantId = data.selectedVariants.length > 1 ? "" : ((data.selected && data.selected.id) || "");
      var $link = $("#product_detail_" + data.product.id + " .product-info__url a");
      if (!$link.length) return;
      var href = $link.attr("href").replace(/&*variant=[a-z0-9-]*/, "");
      $link.attr("href", href + (selectedVariantId ? ((href.indexOf("?") == -1 ? "?" : "&") + "variant=" + selectedVariantId) : ""));
    });
  });
})(window.jQuery);
// 自定义款式 图片上传
$(function () {
  $('.line-item-property__field input[type="file"]').val('');
  $(document).on('change', '.line-item-property__field input[type="file"]', function () {
    var id = $(this).attr('id');
    var file = $(this)[0].files[0];
    file || $('img[data-name="' + id + '"]').attr('src', "").hide();
    if (id && file) {
      if (file.size > (parseInt($("#" + id).data("max-size"), 10) || 10485760)) {
        $('[data-name="' + id + '"],#' + id).val("");
        $('img[data-name="' + id + '"]').attr('src', "").hide();
        return $.toast.show({ content: $("#" + id).data("max-size-msg") || "File size exceeds 10MB, please change to a smaller one" });
      }
      uploadImage(id, file);
    } else {
      $('[data-name="' + id + '"]').val("");
    }
  });
  /**
     * upload image
     */
  function uploadImage(id, file) {
    var objectName = window.SHOP_PARAMS.shop_id + "/" + (new Date()).getTime() + "." + file.name.split(".").pop();
    $("#" + id).css({ "pointer-events": "none" });
    $.getJSON("/api/file/s3-sign?key=" + objectName).then(function (signData) {
      fetch(signData.write_host, {
        method: 'put',
        mode: 'cors',
        body: file,
        headers: { "Content-Type": file.type }
      }).then(function () {
        var url = signData.read_host + objectName;
        $("#" + id).css({ "pointer-events": "" });
        $('[data-name="' + id + '"]').val(url);
        $('img[data-name="' + id + '"]').attr('src', url).show();
        // 修改为已上传
        $('[data-name="' + id + '"]').parent('label');
        $('[data-name="' + id + '"]').parent('.line-item-property__field').find('.not-empty').remove();
      }).catch(function () {
        $("#" + id).css({ "pointer-events": "" });
        $.toast.show({ content: $("#" + id).data("error-msg") || "Upload error, please try again" });
        $('[data-name="' + id + '"],#' + id).val("");
        $('img[data-name="' + id + '"]').attr('src', "").hide();
      });
    });
  }

  $(document).on('click', '.line-item-property__field .item', function () {
    $(this).parents('.line-item-property__field').find('.item').removeClass('selected');
    $(this).parents('.line-item-property__field').find('.item_value').html($(this).find('input').val());
    $(this).addClass('selected');
  })

  $(document).on('blur', '.line-item-property__field .required', function () {
    if ($(this).val()) {
      var $parent = $(this).parents('.line-item-property__field');
      $parent.find('.required').removeClass('not-empty-field');
      $parent.find('.not-empty').remove();
    }
  })
  $('.line-item-property__field').each(function () {
    $(this).find('.item:first').click();
  })
});
/** new product detail ends  */

$(function(){
  if(window.SHOP_PARAMS && window.SHOP_PARAMS.template_type == '1'){
    $(document).on(
      'scroll.view_page_tail',
      $.throttle(
        function () {
          try {
            var scrollTop = $(document).scrollTop(); //滚动条距离顶部的高度
            var clientHeight = $(window).height(); //当前可视的页面高度
            var scrollHeight = $(document).height(); //当前页面的总高度
            //判断是否滑动到底部
            if(scrollHeight - clientHeight <= scrollTop + 10){
              $(document.body).trigger('track', { eventName: 'view_page_tail' })
              $(document).off('scroll.view_page_tail')
            }
          } catch (error) {}
        },
        200,
        50
      )
    );
  }
});
