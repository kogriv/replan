# frozen_string_literal: true

module API
  module V3
    module WikiPages
      class WikiPageListItemRepresenter < ::API::Decorators::Single
        include API::Decorators::LinkedResource
        include API::V3::Workspaces::LinkedResource
        include API::Decorators::DateProperty

        self_link title_getter: ->(*) { represented.title }

        property :id

        property :title

        property :slug

        property :lock_version,
                 render_nil: true,
                 getter: ->(*) { lock_version.to_i }

        property :protected

        date_time_property :created_at
        date_time_property :updated_at

        associated_project

        associated_resource :parent,
                            v3_path: :wiki_page,
                            representer: ::API::V3::WikiPages::WikiPageListItemRepresenter,
                            link_title_attribute: :title,
                            skip_link: ->(*) { represented.parent_id.nil? },
                            getter: ->(*) {}

        associated_resource :author,
                            v3_path: :user,
                            representer: ::API::V3::Users::UserRepresenter,
                            link_title_attribute: :name,
                            skip_link: ->(*) { represented.author_id.nil? },
                            getter: ->(*) {}

        def _type
          "WikiPage"
        end
      end
    end
  end
end
