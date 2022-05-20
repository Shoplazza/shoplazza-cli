/* global $,template */

(function ($) {
  $.header = function (params) {
    var sectionId = params.id;
    var $section = $('[data-section-id="' + sectionId + '"]');
    var $header = $section.find('.header__wrapper');
    var $menu = $section.find('#navigation-pc-menu ul');
    // 菜单点击 -- start
    var $body = $('body');
    // 菜单汉堡
    $body.off('click', '.header__icon_menu').on('click', '.header__icon_menu', function (e) {
      e.preventDefault();
      $('.navigation-m .navigation-m__menu').html($menu.html())
      $('.page_container, .navigation-m').toggleClass('navigation-m_on');
      $('.navigation-m').css('paddingBottom', ($('.navigation-m__setting').height() || 0) + 25 + 'px');
      $section.find('.header__icon_menu i').toggleClass('sep-font-list').toggleClass('sep-font-close');
      $body.toggleClass('no-scroll');
      // iso safari 兼容（fixed点击移动端菜单，关闭按钮无法出现）
      setTimeout(function () {
        $(window).trigger('scroll')
      }, 500);
    });
    $(document).off('click.closeMenu').on('click.closeMenu', function(e){
      if($('.page_container, .navigation-m').is('.navigation-m_on') && !$(e.target).is('.navigation-m, .navigation-m *, .header__icon_menu, .header__icon_menu *')){
        e.stopPropagation();
        e.preventDefault();
        $('.page_container, .navigation-m').toggleClass('navigation-m_on');
        $section.find('.header__icon_menu i').toggleClass('sep-font-list').toggleClass('sep-font-close');
        $body.toggleClass('no-scroll');
      }
    })
    //一级菜单展开
    $body.off('click', '.navigation-m .nav_first-menu-icon').on('click', '.navigation-m .nav_first-menu-icon', function (e) {
      $(this).parent().toggleClass('nav_first-menu-icon-close');
      $(this).toggleClass('sep-font-plus').toggleClass('sep-font-minus');
    });
    //二级菜单展开
    $body.off('click', '.navigation-m .nav_second-menu-icon').on('click', '.navigation-m .nav_second-menu-icon', function (e) {
      $(this).parent().toggleClass('nav_second-menu-icon-close');
      $(this).toggleClass('sep-font-plus').toggleClass('sep-font-minus');
    });
    // B端切换双端处理（关闭菜单）
    if (window.SHOP_PARAMS.shop_env == '1') {
      $(window).resize($.throttle(function () {
        $('.page_container, .navigation-m').removeClass('navigation-m_on');
        $body.removeClass('no-scroll');
        $section.find('.header__icon_menu i').addClass('sep-font-list').removeClass('sep-font-close');
      }, 16, 16))
    }
    // 菜单点击 -- end

    // 三级菜单通屏 -- start
    var timeout;
    $('[data-section-id="header"]').find('.navigation-pc__menu-block_has-child').hover(function() {
      timeout && clearTimeout(timeout);
      $('.menus_container_inner').empty().parent().hide();
      $(this).addClass('navigation-pc__menu-item_hover').siblings().removeClass('navigation-pc__menu-item_hover');
      // 不能直接将append进去会丢失原有dom
      var menuDate = $(this).find('.navigation-hidden-data')
      $('.menus_container_inner').append(menuDate.children().clone()).parent().show();
      $('.menus_container_inner').length && $('.menus_container_inner').css({'max-height': 'calc(100vh - ' + ($('.menus_container_inner').offset().top - $(document).scrollTop()) + 'px)'});
    }, function() {
      timeout = setTimeout(function() {
        $('.menus_container_inner').empty().parent().hide();
        $('.navigation-pc__menu-block_has-child').removeClass('navigation-pc__menu-item_hover');
      },500)
    });
    $('[data-section-id="header"]').find('.menus_container').hover(function() {
      //enter
      clearTimeout(timeout);
      $(document.body).off('mouseleave', '.navigation-pc__menu-block_has-child');
    }, function() {
      //out
      $('.menus_container_inner').empty().parent().hide();
      $('.navigation-pc__menu-block_has-child').removeClass('navigation-pc__menu-item_hover');
    });
    // 三级菜单通屏 -- end

    // 购物车数量 -- start
    (function () {
      // 暴露更新导航购物车数量事件
      $(document).on('dj.common.cart.change update_header_cart', function () {
        $.get('/api/cart/count', function (res) {
          if (res && res.state == "success") {
            // 设置购物车数量
            var count = res.data.count;
            if (count > 99) {
              $('.header__cart-count')
                .html('<span class="header__cart-count_over">99</span>')
                .show();
            } else {
              $('.header__cart-count')
                .html(count)
                .show();
              count == 0 ? $('.header__cart-count').hide() : '';
            }
          }
        });
      });
      // 触发更新购物车数量
      $(document).trigger('update_header_cart');
    })();
    // 购物车数量 -- end

    // 个人中心 -- start
    $body.off('click', '.header__logout').on('click', '.header__logout', function (e) {
      $.ajax({
        type: 'POST',
        dataType: 'json',
        url: '/api/customers/sign_out',
        data: {},
        success: function (data) {
          // 退出登陆埋点
          $(document.body).trigger('dj.logout');
          window.location.reload();
        },
        error: function () {
          window.location.reload();
        }
      });
    });
    // 登陆判断
    // 个人中心 -- end

    // 悬浮 -- start
    (function () {
      var $headerAnnounce = $header.find('.fast-bar');
      // 设置占位符的高度
      var setHeaderPlaceholderHeight = function () {
        return $('[data-section-id="header"]').css('min-height', $header.outerHeight() + 'px')
      }
      params.is_header_fixed && $(window).resize($.throttle(function () {
        setHeaderPlaceholderHeight()
      }, 16, 16));
      $(window).trigger('resize');
      function scrollFix() {
        var headerAnnounceHeight = $headerAnnounce.outerHeight();
        // 判断滚动距离,设置fix定位,填充虚拟dom
        if ($(window).scrollTop() > headerAnnounceHeight) {
          // 顶部缩小
          $headerAnnounce.hide();
          // 悬浮fixed
          $header.addClass('header__fixed');

        } else { // 正常static
          $headerAnnounce.show();
          setHeaderPlaceholderHeight()
          $header.removeClass('header__fixed');
        }
      }
      if (params.is_header_fixed) { // 悬浮fix状态处理
        $(window).on('scroll', window.header_fix = $.throttle(scrollFix, 16, 16));
        window.header_fix();
      } else { // 正常流状态处理
        window.header_fix = null;
        $header.removeClass('fixed-top').css({ boxShadow: 'none', });
      }
    })();
    // 悬浮 -- end

    // 导航后面增加静态插件的div -- start
    (function () {
      // 插件不悬浮（插件放到导航外部）
      $('#shoplaza-section-header').after('<div class="plugin__static-div"></div>');
      // 插件悬浮
      if (params.is_header_fixed) {
        // 导航悬浮（插件放到导航内部）
        $('#shoplaza-section-header').find('.header__wrapper').append('<div class="plugin__fixed-div"></div>');
      } else {
        // 导航不悬浮（插件放到导航外部）
        $('#shoplaza-section-header').after('<div style="position: sticky; top:0; z-index:90; width: 100%;" class="plugin__fixed-div"></div>');
      }
    })();
    // 导航后面增加静态插件的div -- end
  };

})(window.jQuery);