(function ($) {
  $.card_footer = function (params) {
    $("#section_footer").on('click', '#submit_footer_newsletter', function () {
      var emailExp = /^([A-Za-z0-9_\-.])+@([A-Za-z0-9_\-.])+\.([A-Za-z]{2,4})$/g;
      var emailObj = $('#input_email_footer_newsletter');
      var email = emailObj.val();
      var invalidFeedback = $('#footer-newsletter .invalid-feedback');
      var validFeedback = $('#footer-newsletter .valid-feedback');
      var lan = params.lan;
      var feedbacks = {
        'is_empty': lan.email_address_empty_warning,
        'not_formatted': lan.email_address_invalid_warning,
        'is_successful': lan.confirmation
      };

      emailObj.removeClass('is-valid is-invalid');
      invalidFeedback.html('');
      validFeedback.html('');

      if (email === '') {
        invalidFeedback.html(feedbacks['is_empty']);
        emailObj.addClass('is-invalid');
        return;
      }
      if (!emailExp.test(email)) {
        invalidFeedback.html(feedbacks['not_formatted']);
        emailObj.addClass('is-invalid');
        return;
      }
      $('#submit_footer_newsletter').attr("disabled", "");
      $.ajax({
        url: '/api/customers/newsletters',
        type: 'post',
        dataType: 'text',
        data: {
          email: email
        },
        success: function (data, textStatus, jqXHR) {
          if (jqXHR.status === 200) {
            emailObj.addClass('is-valid');
            validFeedback.html(feedbacks['is_successful']);
          }
        },
        error: function (jqXHR) {
          emailObj.addClass('is-invalid');
          invalidFeedback.html(JSON.parse(jqXHR.responseText).errors);
        },
        complete: function () {
          $('#submit_footer_newsletter').removeAttr("disabled");
        }
      });
    }).on('keyup', '#input_email_footer_newsletter', function (e) {
      if (e.keyCode === 13) {
        $('#submit_footer_newsletter').trigger('click');
      }
    });
  };
})(window.jQuery);