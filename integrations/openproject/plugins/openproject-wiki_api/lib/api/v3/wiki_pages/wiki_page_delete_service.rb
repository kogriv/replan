# frozen_string_literal: true

module API
  module V3
    module WikiPages
      class WikiPageDeleteService < ::BaseServices::Delete
        protected

        def default_contract_class
          ::API::V3::WikiPages::WikiPageDeleteContract
        end
      end
    end
  end
end
